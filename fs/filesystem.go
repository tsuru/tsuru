package fs

import (
	"io"
	"os"
)

type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	Stat() (os.FileInfo, error)
}

type Fs interface {
	Create(name string) (File, error)
	Open(name string) (File, error)
	Remove(name string) error
	RemoveAll(path string) error
	Stat(name string) (os.FileInfo, error)
}

type OsFs struct{}

func (fs OsFs) Create(name string) (File, error) {
	return os.Create(name)
}

func (fs OsFs) Open(name string) (File, error) {
	return os.Open(name)
}

func (fs OsFs) Remove(name string) error {
	return os.Remove(name)
}

func (fs OsFs) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs OsFs) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
