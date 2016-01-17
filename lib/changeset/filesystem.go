package changeset

import (
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/osutil"
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
}

var DefaultFilesystem = ExtendedFilesystem{BasicFilesystem{}}

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

// The BasicFilesystem implements all aspects by delegating to package os.
type BasicFilesystem struct{}

func (BasicFilesystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

func (BasicFilesystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
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
