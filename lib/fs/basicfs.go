// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"time"
)

// The BasicFilesystem implements all aspects by delegating to package os.
type BasicFilesystem struct{}

func (BasicFilesystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

func (BasicFilesystem) Mkdir(path string, perm os.FileMode) error {
	return os.Mkdir(path, perm)
}

func (BasicFilesystem) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (BasicFilesystem) Remove(name string) error {
	return os.Remove(name)
}

func (BasicFilesystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (BasicFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (BasicFilesystem) DirNames(path string) ([]string, error) {
	fd, err := os.OpenFile(path, os.O_RDONLY, 0777)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	names, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	return names, nil
}

// OpenWrite returns a writeable file. The file is created if it does not
// exist. If excl is true, O_EXCL is set. If size >= 0, the file is truncated
// to size.
func (BasicFilesystem) OpenWrite(path string, excl bool, size int64) (File, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if excl {
		flags |= os.O_EXCL
	}
	fd, err := os.OpenFile(path, flags, 0666)
	if err != nil {
		return nil, err
	}

	if size >= 0 {
		if err := fd.Truncate(size); err != nil {
			fd.Close()
			return nil, err
		}
	}

	return fd, nil
}
