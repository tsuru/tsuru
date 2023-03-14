// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fstest provides fake implementations of the fs package.
//
// These implementations can be used to mock out the file system in tests.
package fstest

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/tsuru/tsuru/fs"
	"github.com/tsuru/tsuru/safe"
)

// FakeFile represents a fake instance of the File interface.
//
// Methods from FakeFile act like methods in os.File, but instead of working in
// a real file, they work in an internal string.
//
// An instance of FakeFile is returned by RecordingFs.Open method.
type FakeFile struct {
	content string
	current int64
	name    string
	r       *safe.BytesReader
	f       *os.File
	dir     bool
}

func (f *FakeFile) reader() *safe.BytesReader {
	if f.r == nil {
		f.r = safe.NewBytesReader([]byte(f.content))
	}
	return f.r
}

func (f *FakeFile) Close() error {
	atomic.StoreInt64(&f.current, 0)
	if f.f != nil {
		f.f.Close()
		f.f = nil
	}
	return nil
}

func (f *FakeFile) Read(p []byte) (n int, err error) {
	n, err = f.reader().Read(p)
	atomic.AddInt64(&f.current, int64(n))
	return
}

func (f *FakeFile) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = f.reader().ReadAt(p, off)
	atomic.AddInt64(&f.current, off+int64(n))
	return
}

func (f *FakeFile) Seek(offset int64, whence int) (int64, error) {
	ncurrent, err := f.reader().Seek(offset, whence)
	old := atomic.LoadInt64(&f.current)
	for !atomic.CompareAndSwapInt64(&f.current, old, ncurrent) {
		old = atomic.LoadInt64(&f.current)
	}
	return ncurrent, err
}

func (f *FakeFile) Name() string {
	return f.name
}

func (f *FakeFile) Fd() uintptr {
	if f.f == nil {
		var err error
		p := path.Join(os.TempDir(), "testing-fs-file.txt")
		f.f, err = os.Create(p)
		if err != nil {
			panic(err)
		}
	}
	return f.f.Fd()
}

func (f *FakeFile) Stat() (fi os.FileInfo, err error) {
	return &fileInfo{name: f.Name(), size: int64(len(f.content))}, nil
}

func (f *FakeFile) Write(p []byte) (n int, err error) {
	n = len(p)
	cur := atomic.LoadInt64(&f.current)
	currentSize := len(f.content)
	end := int(cur) + n
	if end > currentSize {
		end = currentSize
	}
	diff := cur - int64(currentSize)
	if diff > 0 {
		f.content += strings.Repeat("\x00", int(diff)) + string(p)
	} else {
		f.content = f.content[:cur] + string(p) + f.content[end:]
	}
	return
}

func (f *FakeFile) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}

func (f *FakeFile) Truncate(size int64) error {
	f.content = f.content[:size]
	return nil
}

// RecordingFs implements the Fs interface providing a "recording" file system.
//
// A recording file system is a file system that does not execute any action,
// just record them.
//
// All methods from RecordingFs never return errors.
type RecordingFs struct {
	actions      []string
	actionsMutex sync.Mutex

	files      map[string]*FakeFile
	filesMutex sync.Mutex

	tmpDirCounter uint32

	// FileContent is used to provide content for files opened using
	// RecordingFs.
	FileContent string
}

// HasAction checks if a given action was executed in the filesystem.
//
// For example, when you call the Open method with the "/tmp/file.txt"
// argument, RecordingFs will store locally the action "open /tmp/file.txt" and
// you can check it calling HasAction:
//
//	rfs.Open("/tmp/file.txt")
//	rfs.HasAction("open /tmp/file.txt") // true
func (r *RecordingFs) HasAction(action string) bool {
	r.actionsMutex.Lock()
	defer r.actionsMutex.Unlock()
	for _, a := range r.actions {
		if action == a {
			return true
		}
	}
	return false
}

func (r *RecordingFs) open(name string, read bool) (fs.File, error) {
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files == nil {
		r.files = make(map[string]*FakeFile)
		if r.FileContent == "" && read {
			return nil, syscall.ENOENT
		}
	} else if f, ok := r.files[name]; ok {
		f.r = nil
		return f, nil
	} else if r.FileContent == "" && read {
		return nil, syscall.ENOENT
	}
	fil := &FakeFile{content: r.FileContent, name: name}
	r.files[name] = fil
	return fil, nil
}

// Create records the action "create <name>" and returns an instance of
// FakeFile and nil error.
func (r *RecordingFs) Create(name string) (fs.File, error) {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, "create "+name)
	r.actionsMutex.Unlock()
	return r.open(name, false)
}

// Mkdir records the action "mkdir <name> with mode <perm>" and returns nil.
func (r *RecordingFs) Mkdir(name string, perm os.FileMode) error {
	r.actionsMutex.Lock()
	defer r.actionsMutex.Unlock()
	r.actions = append(r.actions, fmt.Sprintf("mkdir %s with mode %#o", name, perm))
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files == nil {
		r.files = make(map[string]*FakeFile)
	}
	r.files[name] = &FakeFile{name: name, dir: true}
	return nil
}

// MkdirAll records the action "mkdirall <path> with mode <perm>" and returns
// nil.
func (r *RecordingFs) MkdirAll(path string, perm os.FileMode) error {
	r.actionsMutex.Lock()
	defer r.actionsMutex.Unlock()
	r.actions = append(r.actions, fmt.Sprintf("mkdirall %s with mode %#o", path, perm))
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files == nil {
		r.files = make(map[string]*FakeFile)
	}
	r.files[path] = &FakeFile{name: path, dir: true}
	return nil
}

// LastIndexByte from the strings package.
// source: os/tempfile.go
func lastIndex(s string, sep byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return i
		}
	}
	return -1
}

// prefixAndSuffix splits pattern by the last wildcard "*", if applicable,
// returning prefix as the part before "*" and suffix as the part after "*".
// source: os/tempfile.go
func prefixAndSuffix(pattern string) (prefix, suffix string, err error) {
	for i := 0; i < len(pattern); i++ {
		if os.IsPathSeparator(pattern[i]) {
			return "", "", fmt.Errorf("pattern contains path separator")
		}
	}
	if pos := lastIndex(pattern, '*'); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}
	return prefix, suffix, nil
}

// source: os/tempfile.go
func joinPath(dir, name string) string {
	if len(dir) > 0 && os.IsPathSeparator(dir[len(dir)-1]) {
		return dir + name
	}
	return dir + string(os.PathSeparator) + name
}

// MkdirTemp records the action "mkdirtemp into '<dir>' with pattern '<pattern>'"
// (and also the Mkdir()'s "mkdir <name> with mode <perm>")
// and returns the pathname of the new directory.
// If dir="", os.TempDir() is used.
// The pattern follows a 5-digit sequential number (starts at 00001)
// source: os/tempfile.go
func (r *RecordingFs) MkdirTemp(dir string, pattern string) (string, error) {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, fmt.Sprintf("mkdirtemp into '%s' with pattern '%s'", dir, pattern))
	r.actionsMutex.Unlock()

	if dir == "" {
		dir = os.TempDir()
	}

	prefix, suffix, err := prefixAndSuffix(pattern)
	if err != nil {
		return "", &os.PathError{Op: "mkdirtemp", Path: pattern, Err: err}
	}
	prefix = joinPath(dir, prefix)

	r.filesMutex.Lock()
	r.tmpDirCounter++
	name := prefix + fmt.Sprintf("%05d", r.tmpDirCounter) + suffix
	r.filesMutex.Unlock()
	err = r.Mkdir(name, 0700)
	return name, err
}

// Open records the action "open <name>" and returns an instance of FakeFile
// and nil error.
func (r *RecordingFs) Open(name string) (fs.File, error) {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, "open "+name)
	r.actionsMutex.Unlock()
	return r.open(name, true)
}

// OpenFile records the action "openfile <name> with mode <perm>" and returns
// an instance of FakeFile and nil error.
func (r *RecordingFs) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, fmt.Sprintf("openfile %s with mode %#o", name, perm))
	r.actionsMutex.Unlock()
	if flag&os.O_EXCL == os.O_EXCL && flag&os.O_CREATE == os.O_CREATE {
		return nil, syscall.EALREADY
	}
	read := flag&syscall.O_CREAT != syscall.O_CREAT &&
		flag&syscall.O_APPEND != syscall.O_APPEND &&
		flag&syscall.O_TRUNC != syscall.O_TRUNC &&
		flag&syscall.O_WRONLY != syscall.O_WRONLY
	f, err := r.open(name, read)
	if flag&syscall.O_TRUNC == syscall.O_TRUNC {
		f.Truncate(0)
	}
	if flag&syscall.O_APPEND == syscall.O_APPEND {
		f.Seek(0, 2)
	}
	return f, err
}

func (r *RecordingFs) deleteFile(name string) {
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files != nil {
		delete(r.files, name)
	}
}

func (r *RecordingFs) deleteDir(name string) {
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files != nil {
		delete(r.files, name)
	}
}

// Remove records the action "remove <name>" and returns nil.
func (r *RecordingFs) Remove(name string) error {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, "remove "+name)
	r.actionsMutex.Unlock()
	r.deleteFile(name)
	r.deleteDir(name)
	return nil
}

// RemoveAll records the action "removeall <path>" and returns nil.
func (r *RecordingFs) RemoveAll(path string) error {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, "removeall "+path)
	r.actionsMutex.Unlock()
	r.deleteFile(path)
	r.deleteDir(path)
	return nil
}

func (r *RecordingFs) Rename(oldname, newname string) error {
	r.actionsMutex.Lock()
	r.actions = append(r.actions, "rename "+oldname+" "+newname)
	r.actionsMutex.Unlock()
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	if r.files == nil {
		r.files = make(map[string]*FakeFile)
	}

	// rename all dir's childs (very non-performant)
	if r.files[oldname] != nil && r.files[oldname].dir {
		if strings.HasPrefix(newname, oldname) { // moving into itself
			return os.ErrInvalid
		}
		oldDirName := oldname + string(os.PathSeparator)
		for fname := range r.files {
			if suffix := strings.TrimPrefix(fname, oldDirName); suffix != "" && suffix != fname {
				newSubName := path.Join(newname, suffix)
				r.files[newSubName] = r.files[fname]
				delete(r.files, fname)
			}
		}
	}

	r.files[newname] = r.files[oldname]
	delete(r.files, oldname)
	return nil
}

func (r *RecordingFs) Stat(name string) (os.FileInfo, error) {
	r.actionsMutex.Lock()
	defer r.actionsMutex.Unlock()
	r.actions = append(r.actions, "stat "+name)
	file, ok := r.files[name]
	if !ok && r.FileContent == "" {
		return nil, syscall.ENOENT
	}
	info := fileInfo{name: name, size: int64(len(r.FileContent))}
	if file != nil {
		info.size = int64(len(file.content))
	}
	return &info, nil
}

// FileNotFoundFs is like RecordingFs, except that it returns ENOENT on Open,
// OpenFile and Remove.
type FileNotFoundFs struct {
	RecordingFs
}

func (r *FileNotFoundFs) Open(name string) (fs.File, error) {
	r.RecordingFs.Open(name)
	err := os.PathError{Err: syscall.ENOENT, Path: name}
	return nil, &err
}

func (r *FileNotFoundFs) Remove(name string) error {
	r.RecordingFs.Remove(name)
	return &os.PathError{Err: syscall.ENOENT, Path: name}
}

func (r *FileNotFoundFs) RemoveAll(path string) error {
	r.RecordingFs.RemoveAll(path)
	return &os.PathError{Err: syscall.ENOENT, Path: path}
}

func (r *FileNotFoundFs) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	r.RecordingFs.OpenFile(name, flag, perm)
	return r.Open(name)
}

// FailureFs is like RecordingFs, except the it returns an arbitrary error on
// operations.
type FailureFs struct {
	RecordingFs
	Err error
}

func (r *FailureFs) Open(name string) (fs.File, error) {
	r.RecordingFs.Open(name)
	return nil, r.Err
}
