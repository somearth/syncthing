package changeset

import (
	"io"
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
	DirNames(path string) ([]string, error)
	OpenWrite(path string, excl bool, size int64) (WriteOnlyFile, error)
}

type WriteOnlyFile interface {
	io.WriterAt
	io.Closer
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
func (BasicFilesystem) OpenWrite(path string, excl bool, size int64) (WriteOnlyFile, error) {
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
