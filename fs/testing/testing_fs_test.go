// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"errors"
	"github.com/tsuru/tsuru/fs"
	"io/ioutil"
	"launchpad.net/gocheck"
	"os"
	"strings"
	"syscall"
	"testing"
)

type S struct{}

var _ = gocheck.Suite(&S{})

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

func (s *S) TestFakeFilePointerShouldImplementFileInterface(c *gocheck.C) {
	var _ fs.File = &FakeFile{}
}

func (s *S) TestFakeFileClose(c *gocheck.C) {
	f := &FakeFile{content: "doesn't matter"}
	f.current = 500
	err := f.Close()
	c.Assert(err, gocheck.IsNil)
	c.Assert(f.current, gocheck.Equals, int64(0))
}

func (s *S) TestFakeFileCloseInternalFilePointer(c *gocheck.C) {
	f := &FakeFile{}
	f.Fd()
	c.Assert(f.f, gocheck.NotNil)
	f.Close()
	c.Assert(f.f, gocheck.IsNil)
}

func (s *S) TestFakeFileRead(c *gocheck.C) {
	content := "all I am"
	f := &FakeFile{content: content}
	buf := make([]byte, 20)
	n, err := f.Read(buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(content))
	c.Assert(string(buf[:n]), gocheck.Equals, content)
	c.Assert(f.current, gocheck.Equals, int64(len(content)))
}

func (s *S) TestFakeFileReadAt(c *gocheck.C) {
	content := "invisible cage"
	f := &FakeFile{content: content}
	buf := make([]byte, 4)
	n, err := f.ReadAt(buf, int64(len(content)-len(buf)))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 4)
	c.Assert(string(buf), gocheck.Equals, "cage")
	c.Assert(f.current, gocheck.Equals, int64(len(content)))
}

func (s *S) TestFakeFileSeek(c *gocheck.C) {
	content := "fragile equality"
	f := &FakeFile{content: content}
	n, err := f.Seek(8, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, int64(8))
	buf := make([]byte, 5)
	read, err := f.Read(buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(read, gocheck.Equals, 5)
	c.Assert(string(buf), gocheck.Equals, "equal")
}

func (s *S) TestFakeFileFd(c *gocheck.C) {
	f := &FakeFile{}
	defer f.Close()
	fd := f.Fd()
	c.Assert(fd, gocheck.Equals, f.f.Fd())
}

func (s *S) TestFakeFileStat(c *gocheck.C) {
	var empty os.FileInfo
	f := &FakeFile{content: "doesn't matter"}
	fi, err := f.Stat()
	c.Assert(err, gocheck.IsNil)
	c.Assert(fi, gocheck.DeepEquals, empty)
}

func (s *S) TestFakeFileWrite(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Write([]byte("break"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len("break"))
	c.Assert(f.content, gocheck.Equals, "break")
}

func (s *S) TestFakeFileWriteFromPosition(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Seek(5, 0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, int64(5))
	written, err := f.Write([]byte("break"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(written, gocheck.Equals, len("break"))
	c.Assert(f.content, gocheck.Equals, "Guardbreak")
}

func (s *S) TestFakeFileWriteString(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	ret, err := f.WriteString("break")
	c.Assert(err, gocheck.IsNil)
	c.Assert(ret, gocheck.Equals, len("break"))
	c.Assert(f.content, gocheck.Equals, "break")
	f.current = int64(ret)
	ret, err = f.WriteString("break")
	c.Assert(err, gocheck.IsNil)
	c.Assert(ret, gocheck.Equals, len("break"))
	c.Assert(f.content, gocheck.Equals, "breakbreak")
}

func (s *S) TestFakeFileTruncateDoesNotChangeCurrent(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	f.current = 4
	cur := f.current
	err := f.Truncate(0)
	c.Assert(err, gocheck.IsNil)
	c.Assert(f.current, gocheck.Equals, cur)
}

func (s *S) TestFakeFileTruncateStripsContentWithN(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	err := f.Truncate(4)
	c.Assert(err, gocheck.IsNil)
	c.Assert(f.content, gocheck.Equals, "Guar")
}

func (s *S) TestFakeFileTruncateWithoutSeek(c *gocheck.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	f.current = int64(len(content))
	err := f.Truncate(0)
	c.Assert(err, gocheck.IsNil)
	tow := []byte("otherthing")
	n, err := f.Write(tow)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(tow))
	nulls := strings.Repeat("\x00", int(f.current))
	c.Assert(f.content, gocheck.Equals, nulls+string(tow))
}

func (s *S) TestRecordingFsPointerShouldImplementFsInterface(c *gocheck.C) {
	var _ fs.Fs = &RecordingFs{}
}

func (s *S) TestRecordingFsHasAction(c *gocheck.C) {
	fs := RecordingFs{actions: []string{"torn", "shade of my soul"}}
	c.Assert(fs.HasAction("torn"), gocheck.Equals, true)
	c.Assert(fs.HasAction("shade of my soul"), gocheck.Equals, true)
	c.Assert(fs.HasAction("meaningles world"), gocheck.Equals, false)
}

func (s *S) TestRecordingFsCreate(c *gocheck.C) {
	fs := RecordingFs{}
	f, err := fs.Create("/my/file.txt")
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("create /my/file.txt"), gocheck.Equals, true)
	c.Assert(f, gocheck.FitsTypeOf, &FakeFile{})
}

func (s *S) TestRecordingFsMkdir(c *gocheck.C) {
	fs := RecordingFs{}
	err := fs.Mkdir("/my/dir", 0777)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("mkdir /my/dir with mode 0777"), gocheck.Equals, true)
}

func (s *S) TestRecordingFsMkdirAll(c *gocheck.C) {
	fs := RecordingFs{}
	err := fs.MkdirAll("/my/dir/with/subdir", 0777)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("mkdirall /my/dir/with/subdir with mode 0777"), gocheck.Equals, true)
}

func (s *S) TestRecordingFsOpen(c *gocheck.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.Open("/my/file")
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("open /my/file"), gocheck.Equals, true)
	c.Assert(f, gocheck.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, gocheck.Equals, fs.FileContent)
}

func (s *S) TestRecordingFsOpenFile(c *gocheck.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.OpenFile("/my/file", 0, 0600)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("openfile /my/file with mode 0600"), gocheck.Equals, true)
	c.Assert(f, gocheck.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, gocheck.Equals, fs.FileContent)
}

func (s *S) TestRecordingFsOpenFileTruncate(c *gocheck.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.OpenFile("/my/file", syscall.O_TRUNC, 0600)
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("openfile /my/file with mode 0600"), gocheck.Equals, true)
	c.Assert(f, gocheck.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, gocheck.Equals, "")
}

func (s *S) TestRecordingFsOpenFileAppend(c *gocheck.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", syscall.O_APPEND|syscall.O_WRONLY, 0644)
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("Hi there!\n"))
	f.Close()
	f, err = fs.OpenFile("/my/file", syscall.O_APPEND|syscall.O_WRONLY, 0644)
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("Hi there!\n"))
	f.Close()
	f, err = fs.Open("/my/file")
	c.Assert(err, gocheck.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "Hi there!\nHi there!\n")
}

func (s *S) TestRecordingFsOpenFileCreateAndExclusive(c *gocheck.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", os.O_EXCL|os.O_CREATE, 0600)
	c.Assert(err, gocheck.Equals, syscall.EALREADY)
	c.Assert(f, gocheck.IsNil)
}

func (s *S) TestRecordingFsOpenFileReadAndWriteENOENT(c *gocheck.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", syscall.O_RDWR, 0600)
	c.Assert(f, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, syscall.ENOENT)
}

func (s *S) TestRecordingFsKeepFileInstances(c *gocheck.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.Create("/my/file")
	c.Assert(err, gocheck.IsNil)
	f.Write([]byte("hi"))
	f, err = fs.Open("/my/file")
	c.Assert(err, gocheck.IsNil)
	buf := make([]byte, 2)
	n, err := f.Read(buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 2)
	c.Assert(string(buf), gocheck.Equals, "hi")
	// Opening again should read seek to position 0 in the reader
	f, _ = fs.Open("/my/file")
	n, err = f.Read(buf)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, 2)
	c.Assert(string(buf), gocheck.Equals, "hi")
}

func (s *S) TestRecordingFsShouldKeepWrittenContent(c *gocheck.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, _ := fs.Open("/my/file")
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	f.Close()
	c.Assert(string(buf[:n]), gocheck.Equals, "the content")
	f, _ = fs.Create("/my/file")
	f.Write([]byte("content the"))
	f.Close()
	f, _ = fs.Open("/my/file")
	n, _ = f.Read(buf)
	c.Assert(string(buf[:n]), gocheck.Equals, "content the")
}

func (s *S) TestRecordingFsFailToOpenUnknownFilesWithoutContent(c *gocheck.C) {
	fs := RecordingFs{}
	f, err := fs.Open("/my/file")
	c.Assert(f, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(os.IsNotExist(err), gocheck.Equals, true)
}

func (s *S) TestRecordingFsRemove(c *gocheck.C) {
	fs := RecordingFs{}
	err := fs.Remove("/my/file")
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("remove /my/file"), gocheck.Equals, true)
}

func (s *S) TestRecordingFsRemoveDeletesState(c *gocheck.C) {
	fs := RecordingFs{FileContent: "hi"}
	f, _ := fs.Open("/my/file")
	f.Write([]byte("ih"))
	fs.Remove("/my/file")
	f, _ = fs.Open("/my/file")
	buf := make([]byte, 2)
	f.Read(buf)
	c.Assert(string(buf), gocheck.Equals, "hi")
}

func (s *S) TestRecordingFsRemoveAll(c *gocheck.C) {
	fs := RecordingFs{}
	err := fs.RemoveAll("/my/dir")
	c.Assert(err, gocheck.IsNil)
	c.Assert(fs.HasAction("removeall /my/dir"), gocheck.Equals, true)
}

func (s *S) TestRecordingFsRemoveAllDeletesState(c *gocheck.C) {
	fs := RecordingFs{FileContent: "hi"}
	f, _ := fs.Open("/my/file")
	f.Write([]byte("ih"))
	fs.RemoveAll("/my/file")
	f, _ = fs.Open("/my/file")
	buf := make([]byte, 2)
	f.Read(buf)
	c.Assert(string(buf), gocheck.Equals, "hi")
}

func (s *S) TestRecordingFsRename(c *gocheck.C) {
	fs := RecordingFs{}
	f, _ := fs.Create("/my/file")
	f.Write([]byte("hello, hello!"))
	f.Close()
	err := fs.Rename("/my/file", "/your/file")
	c.Assert(err, gocheck.IsNil)
	_, err = fs.Open("/my/file")
	c.Assert(err, gocheck.NotNil)
	f, err = fs.Open("/your/file")
	c.Assert(err, gocheck.IsNil)
	defer f.Close()
	b, _ := ioutil.ReadAll(f)
	c.Assert(string(b), gocheck.Equals, "hello, hello!")
	c.Assert(fs.HasAction("rename /my/file /your/file"), gocheck.Equals, true)
}

func (s *S) TestRecordingFsCold(c *gocheck.C) {
	fs := RecordingFs{}
	err := fs.Rename("/my/file", "/your/file")
	c.Assert(err, gocheck.IsNil)
}

func (s *S) TestRecordingFsStat(c *gocheck.C) {
	fs := RecordingFs{}
	fi, err := fs.Stat("/my/file")
	c.Assert(err, gocheck.IsNil)
	c.Assert(fi, gocheck.IsNil)
	c.Assert(fs.HasAction("stat /my/file"), gocheck.Equals, true)
}

func (s *S) TestFileNotFoundFsPointerImplementsFsInterface(c *gocheck.C) {
	var _ fs.Fs = &FileNotFoundFs{}
}

func (s *S) TestFileNotFoundFsOpen(c *gocheck.C) {
	fs := FileNotFoundFs{}
	f, err := fs.Open("/my/file")
	c.Assert(f, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, gocheck.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, gocheck.Equals, "/my/file")
	c.Assert(fs.HasAction("open /my/file"), gocheck.Equals, true)
}

func (s *S) TestFileNotFoundFsRemove(c *gocheck.C) {
	fs := FileNotFoundFs{}
	err := fs.Remove("/my/file")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, gocheck.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, gocheck.Equals, "/my/file")
	c.Assert(fs.HasAction("remove /my/file"), gocheck.Equals, true)
}

func (s *S) TestFileNotFoundFsOpenFile(c *gocheck.C) {
	fs := FileNotFoundFs{}
	f, err := fs.OpenFile("/my/file", 0, 0600)
	c.Assert(f, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, gocheck.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, gocheck.Equals, "/my/file")
	c.Assert(fs.HasAction("open /my/file"), gocheck.Equals, true)
}

func (s *S) TestFileNotFoundFsRemoveAll(c *gocheck.C) {
	fs := FileNotFoundFs{}
	err := fs.RemoveAll("/my/file")
	c.Assert(err, gocheck.NotNil)
	c.Assert(err, gocheck.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, gocheck.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, gocheck.Equals, "/my/file")
	c.Assert(fs.HasAction("removeall /my/file"), gocheck.Equals, true)
}

func (s *S) TestFailureFsOpen(c *gocheck.C) {
	origErr := errors.New("something went wrong")
	fs := FailureFs{Err: origErr}
	file, gotErr := fs.Open("/wat")
	c.Assert(file, gocheck.IsNil)
	c.Assert(gotErr, gocheck.Equals, origErr)
}
