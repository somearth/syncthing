// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

// When chmodding, retain these bits if they are set
const retainBits = os.ModeSetuid | os.ModeSetgid | os.ModeSticky

func (c *ChangeSet) writeDir(d protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, d.Name)
	mode := os.FileMode(d.Flags & 0777)
	if d.Flags&protocol.FlagNoPermBits != 0 {
		// Ignore permissions is set, so use a default set of bits (modified
		// by umask later on)
		mode = 0777
	}

	// Check if the directory already exists, if so just set the permissions, unless we shouldn't do that either
	if info, err := c.Filesystem.Lstat(realPath); err == nil && info.IsDir() {
		if d.Flags&protocol.FlagNoPermBits == 0 {
			mode = mode | info.Mode()&retainBits
			if err = c.Filesystem.Chmod(realPath, mode); err != nil {
				return &opError{file: d.Name, op: "writeDir Chmod", err: err}
			}
		}
		return nil
	}

	// First we try a normal MkdirAll which will attempt to create all the
	// directories up to this point.
	err := c.Filesystem.MkdirAll(realPath, mode)
	if os.IsPermission(err) {
		// If that failed due to permissions error, we retry with an
		// InWritableDir call. This succeeds if we were trying to mkdir
		// "foo/bar" and "foo" is read only, but it fails if we try to mkdir
		// "foo/bar/baz", "foo/bar" doesn't exist and "foo" is readonly. That
		// should be unusual...
		err = osutil.InWritableDir(func(path string) error {
			return c.Filesystem.MkdirAll(path, mode)
		}, realPath)
	}

	if err == nil {
		err = c.Filesystem.Chmod(realPath, mode)
	}

	if err != nil {
		return &opError{file: d.Name, op: "writeDir MkdirAll", err: err}
	}

	return nil
}

func (c *ChangeSet) deleteDir(d protocol.FileInfo) *opError {
	realPath := filepath.Join(c.rootPath, d.Name)
	if _, err := c.Filesystem.Lstat(realPath); err != nil {
		// Things that we can't stat don't exist
		return nil
	}
	if err := osutil.InWritableDir(c.Filesystem.Remove, realPath); err != nil {
		return &opError{file: d.Name, op: "deleteDir Remove", err: err}
	}

	return nil
}
