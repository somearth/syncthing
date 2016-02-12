// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
	"os"
	"time"
)

// The Filesystem interface abstracts access to the file system.
type Filesystem interface {
	Chmod(name string, mode os.FileMode) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Lstat(name string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	Remove(name string) error
	Rename(oldpath, newpath string) error
	Stat(name string) (os.FileInfo, error)
	DirNames(path string) ([]string, error)
	OpenWrite(path string, excl bool, size int64) (WriteOnlyFile, error)
}

type WriteOnlyFile interface {
	io.WriterAt
	io.Closer
}

var DefaultFilesystem = ExtendedFilesystem{BasicFilesystem{}}
