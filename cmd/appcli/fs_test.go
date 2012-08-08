package main

import (
	"github.com/timeredbull/tsuru/fs"
	"os"
	"strings"
	"syscall"
)

type fakeFile struct {
	content string
	r       *strings.Reader
}

func (f *fakeFile) reader() *strings.Reader {
	if f.r == nil {
		f.r = strings.NewReader(f.content)
	}
	return f.r
}

func (f *fakeFile) Close() error {
	return nil
}

func (f *fakeFile) Read(p []byte) (n int, err error) {
	return f.reader().Read(p)
}

func (f *fakeFile) ReadAt(p []byte, off int64) (n int, err error) {
	return f.reader().ReadAt(p, off)
}

func (f *fakeFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader().Seek(offset, whence)
}

func (f *fakeFile) Stat() (fi os.FileInfo, err error) {
	return
}

type RecordingFs struct {
	actions     []string
	fileContent string
}

func (r *RecordingFs) HasAction(action string) bool {
	for _, a := range r.actions {
		if action == a {
			return true
		}
	}
	return false
}

func (r *RecordingFs) Create(name string) (fs.File, error) {
	return nil, nil
}

func (r *RecordingFs) Mkdir(name string, perm os.FileMode) error {
	return nil
}

func (r *RecordingFs) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (r *RecordingFs) Open(name string) (fs.File, error) {
	r.actions = append(r.actions, "open "+name)
	fil := fakeFile{content: r.fileContent}
	return &fil, nil
}

func (r *RecordingFs) Remove(name string) error {
	r.actions = append(r.actions, "remove "+name)
	return nil
}

func (r *RecordingFs) RemoveAll(path string) error {
	r.actions = append(r.actions, "removeall "+path)
	return nil
}

func (r *RecordingFs) Stat(name string) (os.FileInfo, error) {
	return nil, nil
}

type FailureFs struct {
	RecordingFs
}

func (r *FailureFs) Open(name string) (fs.File, error) {
	err := os.PathError{
		Err: syscall.ENOENT,
	}
	return nil, &err
}
