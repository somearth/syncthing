// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"time"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

func (c *ChangeSet) writeFile(f protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, f.Name)
	tempPath := c.TempNamer.TempName(realPath)

	inConflict := false
	if c.CurrentFiler != nil {
		if curFile, ok := c.CurrentFiler.CurrentFile(f.Name); ok {
			// We have a CurrentFiler and it returned an existing file for
			// the one we are about to replace.

			// Check that the modification time and size matches, otherwise
			// return an error indicating that we need a rescan before we can
			// do this change.
			if info, err := c.Filesystem.Lstat(realPath); err == nil {
				var mismatch error
				if info.ModTime().Unix() != curFile.Modified {
					mismatch = fmt.Errorf("modification time mismatch")
				} else if info.Size() != curFile.Size() {
					mismatch = fmt.Errorf("size mismatch")
				}
				if mismatch != nil {
					return &opError{
						file:       f.Name,
						op:         "writeFile check",
						err:        mismatch,
						mustRescan: true,
					}
				}
			}

			// Check if we are in conflict (i.e., the file has been
			// concurrently modified).
			inConflict = curFile.Version.Concurrent(f.Version)

			// Check if the hash of the existing file is equal to the current
			// one. If so, we just update the metadata.

			// TODO: Implement shortcut

		}
	}

	reuse := false
	fileSize := f.Size() // We might override f.Blocks when reusing

	if _, err := c.Filesystem.Lstat(tempPath); err == nil {
		// Temporary file already exists. Reuse blocks from it.
		blocks, err := scanner.HashFile(tempPath, protocol.BlockSize, 0, nil)
		if err != nil {
			// Weird, we couldn't scan the temp file. Try to remove it?
			if err := osutil.InWritableDir(c.Filesystem.Remove, tempPath); err != nil {
				// Dammit :(
				return &opError{file: f.Name, op: "writeFile remove reused temp file", err: err}
			}
		} else {
			// Now we only need to grab the blocks that are missing from the
			// temp file.
			_, f.Blocks = scanner.BlockDiff(blocks, f.Blocks)
			reuse = true
		}
	}

	fd, err := c.openTempFile(tempPath, reuse, fileSize)
	if err != nil {
		return &opError{file: f.Name, op: "writeFile open", err: err}
	}

	buf := make([]byte, protocol.BlockSize)
	errC := make(chan *opError, 1)
	var wg sync.WaitGroup

	for _, block := range f.Blocks {
		select {
		case err := <-errC:
			// An error has ocurred on a background request. We should abort processing.
			wg.Wait() // wait for background requests to finish
			fd.Close()
			return err
		default:
		}

		if block.IsEmpty() && !reuse {
			// The block is a block of all zeroes, and we are not reusing
			// a temp file, so there is no need to do anything with it.
			// If we were reusing a temp file and had this block to copy,
			// it would be because the block in the temp file was *not* a
			// block of all zeroes, so then we should not skip it.

			// Pretend we copied it.
			c.Progresser.Progress(f, int(block.Size), 0, 0)
			continue
		}

		buf = buf[:block.Size]

		err := fmt.Errorf("no source") // will be overwritten if there are any sources to talk to
		if c.LocalRequester != nil {
			// Try to reuse the block from somewhere local.
			err = c.LocalRequester.Request(f.Name, block.Offset, block.Hash, buf)
			if err == nil {
				// Write out the block, returning early on failure.
				_, err = fd.WriteAt(buf, block.Offset)
				if err != nil {
					fd.Close()
					return &opError{file: f.Name, op: "writeFile write", err: err}
				}

				if c.Progresser != nil {
					// Tell the Progresser that we copied a block.
					c.Progresser.Progress(f, int(block.Size), 0, 0)
				}
			}
		}

		if err != nil && c.NetworkRequester != nil {
			// We got an error from the local source, try to request it from
			// the network instead.
			resp := c.NetworkRequester.Request(f.Name, block.Offset, block.Hash, int(block.Size))
			err = nil // The background request succeeded; if it fails, that'll be handled the errC route.
			wg.Add(1)
			if c.Progresser != nil {
				// Tell the Progresser that we requested a block.
				c.Progresser.Progress(f, 0, int(block.Size), 0)
			}
			go func(block protocol.BlockInfo) {
				defer resp.Close()
				defer wg.Done()

				err := resp.Error()
				if err != nil {
					if c.Progresser != nil {
						// Tell the Progresser that the download failed
						c.Progresser.Progress(f, 0, -int(block.Size), 0)
					}
					// Try to send the error on the errC. Do this with a
					// select as something else may already have errored out
					// and is blocking the errC.
					select {
					case errC <- &opError{file: f.Name, op: "background request", err: err}:
					default:
					}
					return
				}

				_, err = resp.WriteAt(fd, block.Offset)
				if err != nil {
					if c.Progresser != nil {
						// Tell the Progresser that the download failed
						c.Progresser.Progress(f, 0, -int(block.Size), 0)
					}
					select {
					case errC <- &opError{file: f.Name, op: "background write", err: err}:
					default:
					}
					return
				}

				if c.Progresser != nil {
					// Tell the Progresser that we downloaded a block.
					c.Progresser.Progress(f, 0, -int(block.Size), int(block.Size))
				}
			}(block)
		}

		if err != nil {
			wg.Wait() // wait for background requests to finish
			fd.Close()
			return &opError{file: f.Name, op: "pull", err: err}
		}
	}

	// Wait for all background network requests to complete.
	wg.Wait()

	// Check for an error from those requests.
	select {
	case err := <-errC:
		fd.Close()
		return err
	default:
	}

	// Close the temporary file, returning on error.
	if err := fd.Close(); err != nil {
		return &opError{file: f.Name, op: "writeFile close", err: err}
	}

	if f.Flags&protocol.FlagNoPermBits == 0 {
		if err := c.Filesystem.Chmod(tempPath, os.FileMode(f.Flags&0777)); err != nil {
			return &opError{file: f.Name, op: "writeFile chmod", err: err}
		}
	}

	modTime := time.Unix(f.Modified, 0)
	if err := c.Filesystem.Chtimes(tempPath, modTime, modTime); err != nil {
		return &opError{file: f.Name, op: "writeFile chtimes", err: err}
	}

	if inConflict {
		c.moveForConflict(realPath)
	} else if c.Archiver != nil {
		c.Archiver.Archive(realPath)
	}
	if err := c.Filesystem.Rename(tempPath, realPath); err != nil {
		return &opError{file: f.Name, op: "writeFile rename", err: err}
	}

	return nil
}

func (c *ChangeSet) deleteFile(f protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, f.Name)
	if c.Archiver != nil {
		c.Archiver.Archive(realPath)
	}
	if err := osutil.InWritableDir(c.Filesystem.Remove, realPath); err != nil {
		if os.IsNotExist(err) {
			// Things that don't exist are removed
			return nil
		}
		if _, err := c.Filesystem.Lstat(realPath); err != nil {
			// Things that we can't stat don't exist
			return nil
		}
		// All other errors are legit
		return &opError{file: f.Name, op: "deleteFile remove", err: err}
	}

	return nil
}

func (c *ChangeSet) renameFile(from, to protocol.FileInfo) *opError {
	realFrom := filepath.Join(c.rootPath, from.Name)
	realTo := filepath.Join(c.rootPath, to.Name)
	if err := c.Filesystem.Rename(realFrom, realTo); err != nil {
		return &opError{file: to.Name, op: "renameFile", err: err}
	}
	return nil
}

func (c *ChangeSet) renameFileFrom(from protocol.FileInfo) func(to protocol.FileInfo) *opError {
	return func(to protocol.FileInfo) *opError {
		return c.renameFile(from, to)
	}
}

func (c *ChangeSet) moveForConflict(name string) error {
	if strings.Contains(filepath.Base(name), ".sync-conflict-") {
		return nil
	}
	if c.maxConflicts == 0 {
		if err := osutil.InWritableDir(c.Filesystem.Remove, name); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	ext := filepath.Ext(name) // includes dot
	withoutExt := name[:len(name)-len(ext)]
	newName := withoutExt + time.Now().Format(".sync-conflict-20060102-150405") + ext
	err := c.Filesystem.Rename(name, newName)
	if os.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}

	if c.maxConflicts > -1 {
		// We list existing conflict files here by Readdirnames instead of
		// using os.Glob for because os.Glob has problems when the base
		// filename contains characters that are valid glob characters ([] and
		// so on).

		dir := filepath.Dir(name)
		names, dirErr := c.Filesystem.DirNames(dir)
		if dirErr != nil {
			// Return the result of rename from above; never mind this failure
			return err
		}

		prefix := withoutExt + ".sync-conflict-"
		var matches []string
		for _, name := range names {
			// We check if the file name starts with the expected prefix and
			// it has the correct extension.
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ext) {
				matches = append(matches, name)
			}
		}

		if len(matches) > c.maxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[c.maxConflicts:] {
				c.Filesystem.Remove(filepath.Join(dir, match)) // if we fail, we fail
			}
		}
	}

	return err
}

func (c *ChangeSet) openTempFile(tempPath string, reuse bool, size int64) (WriteOnlyFile, error) {
	excl := false

	if !reuse {
		excl = true

		// Ensure that the parent directory is writable. This is
		// osutil.InWritableDir except we need to do more stuff so we
		// duplicate it here.
		dir := filepath.Dir(tempPath)
		if info, err := c.Filesystem.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				if err := c.Filesystem.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("openTempFile: revive dir: %v", err)
				}
			} else {
				return nil, fmt.Errorf("openTempFile: stat dir: %v", err)
			}
		} else if info.Mode()&0200 == 0 {
			if err := c.Filesystem.Chmod(dir, 0755); err == nil {
				defer c.Filesystem.Chmod(dir, info.Mode().Perm())
			}
		}
	} else {
		// With sufficiently bad luck when exiting or crashing, we may have
		// had time to chmod the temp file to read only state but not yet
		// moved it to it's final name. This leaves us with a read only temp
		// file that we're going to try to reuse. To handle that, we need to
		// make sure we have write permissions on the file before opening it.
		// Ignore the error here as we may be on a filesystem that doesn't
		// support it and we'll fail nicely on the next operation anyhow.
		c.Filesystem.Chmod(tempPath, 0666)
	}

	fd, err := c.Filesystem.OpenWrite(tempPath, excl, size)
	if err != nil {
		return nil, fmt.Errorf("openTempFile: open (reuse=%v): %v", reuse, err)
	}

	return fd, nil
}
