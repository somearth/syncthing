// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"os"

	"github.com/syncthing/syncthing/lib/osutil"
)

// The ExtendedFilesystem implements some methods by delegating to package
// osutil, thereby working around some operating specific issues.
type ExtendedFilesystem struct {
	Filesystem
}

func (ExtendedFilesystem) MkdirAll(path string, perm os.FileMode) error {
	return osutil.MkdirAll(path, perm)
}

func (ExtendedFilesystem) Remove(name string) error {
	return osutil.Remove(name)
}

func (ExtendedFilesystem) Rename(oldpath, newpath string) error {
	return osutil.Rename(oldpath, newpath)
}
