package fs

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

type FakeFile struct {
	content string
	r       *strings.Reader
}

func (f *FakeFile) reader() *strings.Reader {
	if f.r == nil {
		f.r = strings.NewReader(f.content)
	}
	return f.r
}

func (f *FakeFile) Close() error {
	return nil
}

func (f *FakeFile) Read(p []byte) (n int, err error) {
	return f.reader().Read(p)
}

func (f *FakeFile) ReadAt(p []byte, off int64) (n int, err error) {
	return f.reader().ReadAt(p, off)
}

func (f *FakeFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader().Seek(offset, whence)
}

func (f *FakeFile) Stat() (fi os.FileInfo, err error) {
	return
}

type RecordingFs struct {
	actions     []string
	FileContent string
}

func (r *RecordingFs) HasAction(action string) bool {
	for _, a := range r.actions {
		if action == a {
			return true
		}
	}
	return false
}

func (r *RecordingFs) Create(name string) (File, error) {
	r.actions = append(r.actions, "create "+name)
	fil := FakeFile{content: r.FileContent}
	return &fil, nil
}

func (r *RecordingFs) Mkdir(name string, perm os.FileMode) error {
	r.actions = append(r.actions, fmt.Sprintf("mkdir %s with mode %#o", name, perm))
	return nil
}

func (r *RecordingFs) MkdirAll(path string, perm os.FileMode) error {
	r.actions = append(r.actions, fmt.Sprintf("mkdirall %s with mode %#o", path, perm))
	return nil
}

func (r *RecordingFs) Open(name string) (File, error) {
	r.actions = append(r.actions, "open "+name)
	fil := FakeFile{content: r.FileContent}
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
	r.actions = append(r.actions, "stat "+name)
	return nil, nil
}

type FailureFs struct {
	RecordingFs
}

func (r *FailureFs) Open(name string) (File, error) {
	r.RecordingFs.Open(name)
	err := os.PathError{
		Err: syscall.ENOENT,
	}
	return nil, &err
}
