// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fstest

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/tsuru/tsuru/fs"
	"gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) TestFakeFilePointerShouldImplementFileInterface(c *check.C) {
	var _ fs.File = &FakeFile{}
}

func (s *S) TestFakeFileClose(c *check.C) {
	f := &FakeFile{content: "doesn't matter"}
	f.current = 500
	err := f.Close()
	c.Assert(err, check.IsNil)
	c.Assert(f.current, check.Equals, int64(0))
}

func (s *S) TestFakeFileCloseInternalFilePointer(c *check.C) {
	f := &FakeFile{}
	f.Fd()
	c.Assert(f.f, check.NotNil)
	f.Close()
	c.Assert(f.f, check.IsNil)
}

func (s *S) TestFakeFileRead(c *check.C) {
	content := "all I am"
	f := &FakeFile{content: content}
	buf := make([]byte, 20)
	n, err := f.Read(buf)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(content))
	c.Assert(string(buf[:n]), check.Equals, content)
	c.Assert(f.current, check.Equals, int64(len(content)))
}

func (s *S) TestFakeFileReadAt(c *check.C) {
	content := "invisible cage"
	f := &FakeFile{content: content}
	buf := make([]byte, 4)
	n, err := f.ReadAt(buf, int64(len(content)-len(buf)))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 4)
	c.Assert(string(buf), check.Equals, "cage")
	c.Assert(f.current, check.Equals, int64(len(content)))
}

func (s *S) TestFakeFileSeek(c *check.C) {
	content := "fragile equality"
	f := &FakeFile{content: content}
	n, err := f.Seek(8, 0)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(8))
	buf := make([]byte, 5)
	read, err := f.Read(buf)
	c.Assert(err, check.IsNil)
	c.Assert(read, check.Equals, 5)
	c.Assert(string(buf), check.Equals, "equal")
}

func (s *S) TestFakeFileFd(c *check.C) {
	f := &FakeFile{}
	defer f.Close()
	fd := f.Fd()
	c.Assert(fd, check.Equals, f.f.Fd())
}

func (s *S) TestFakeFileName(c *check.C) {
	var f FakeFile
	f.name = "/home/user/.bash_profile"
	defer f.Close()
	c.Assert(f.Name(), check.Equals, f.name)
}

func (s *S) TestFakeFileStat(c *check.C) {
	var empty os.FileInfo
	f := &FakeFile{content: "doesn't matter"}
	fi, err := f.Stat()
	c.Assert(err, check.IsNil)
	c.Assert(fi, check.DeepEquals, empty)
}

func (s *S) TestFakeFileWrite(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Write([]byte("break"))
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len("break"))
	c.Assert(f.content, check.Equals, "break")
}

func (s *S) TestFakeFileWriteFromPosition(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Seek(5, 0)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, int64(5))
	written, err := f.Write([]byte("break"))
	c.Assert(err, check.IsNil)
	c.Assert(written, check.Equals, len("break"))
	c.Assert(f.content, check.Equals, "Guardbreak")
}

func (s *S) TestFakeFileWriteString(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	ret, err := f.WriteString("break")
	c.Assert(err, check.IsNil)
	c.Assert(ret, check.Equals, len("break"))
	c.Assert(f.content, check.Equals, "break")
	f.current = int64(ret)
	ret, err = f.WriteString("break")
	c.Assert(err, check.IsNil)
	c.Assert(ret, check.Equals, len("break"))
	c.Assert(f.content, check.Equals, "breakbreak")
}

func (s *S) TestFakeFileTruncateDoesNotChangeCurrent(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	f.current = 4
	cur := f.current
	err := f.Truncate(0)
	c.Assert(err, check.IsNil)
	c.Assert(f.current, check.Equals, cur)
}

func (s *S) TestFakeFileTruncateStripsContentWithN(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	err := f.Truncate(4)
	c.Assert(err, check.IsNil)
	c.Assert(f.content, check.Equals, "Guar")
}

func (s *S) TestFakeFileTruncateWithoutSeek(c *check.C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	f.current = int64(len(content))
	err := f.Truncate(0)
	c.Assert(err, check.IsNil)
	tow := []byte("otherthing")
	n, err := f.Write(tow)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(tow))
	nulls := strings.Repeat("\x00", int(f.current))
	c.Assert(f.content, check.Equals, nulls+string(tow))
}

func (s *S) TestRecordingFsPointerShouldImplementFsInterface(c *check.C) {
	var _ fs.Fs = &RecordingFs{}
}

func (s *S) TestRecordingFsHasAction(c *check.C) {
	fs := RecordingFs{actions: []string{"torn", "shade of my soul"}}
	c.Assert(fs.HasAction("torn"), check.Equals, true)
	c.Assert(fs.HasAction("shade of my soul"), check.Equals, true)
	c.Assert(fs.HasAction("meaningles world"), check.Equals, false)
}

func (s *S) TestRecordingFsCreate(c *check.C) {
	fs := RecordingFs{}
	f, err := fs.Create("/my/file.txt")
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("create /my/file.txt"), check.Equals, true)
	c.Assert(f, check.FitsTypeOf, &FakeFile{})
}

func (s *S) TestRecordingFsMkdir(c *check.C) {
	fs := RecordingFs{}
	err := fs.Mkdir("/my/dir", 0777)
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("mkdir /my/dir with mode 0777"), check.Equals, true)
}

func (s *S) TestRecordingFsMkdirAll(c *check.C) {
	fs := RecordingFs{}
	err := fs.MkdirAll("/my/dir/with/subdir", 0777)
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("mkdirall /my/dir/with/subdir with mode 0777"), check.Equals, true)
}

func (s *S) TestRecordingFsOpen(c *check.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.Open("/my/file")
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("open /my/file"), check.Equals, true)
	c.Assert(f, check.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, check.Equals, fs.FileContent)
}

func (s *S) TestRecordingFsOpenFile(c *check.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.OpenFile("/my/file", 0, 0600)
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("openfile /my/file with mode 0600"), check.Equals, true)
	c.Assert(f, check.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, check.Equals, fs.FileContent)
}

func (s *S) TestRecordingFsOpenFileTruncate(c *check.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.OpenFile("/my/file", syscall.O_TRUNC, 0600)
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("openfile /my/file with mode 0600"), check.Equals, true)
	c.Assert(f, check.FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, check.Equals, "")
}

func (s *S) TestRecordingFsOpenFileAppend(c *check.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", syscall.O_APPEND|syscall.O_WRONLY, 0644)
	c.Assert(err, check.IsNil)
	f.Write([]byte("Hi there!\n"))
	f.Close()
	f, err = fs.OpenFile("/my/file", syscall.O_APPEND|syscall.O_WRONLY, 0644)
	c.Assert(err, check.IsNil)
	f.Write([]byte("Hi there!\n"))
	f.Close()
	f, err = fs.Open("/my/file")
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, "Hi there!\nHi there!\n")
}

func (s *S) TestRecordingFsOpenFileCreateAndExclusive(c *check.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", os.O_EXCL|os.O_CREATE, 0600)
	c.Assert(err, check.Equals, syscall.EALREADY)
	c.Assert(f, check.IsNil)
}

func (s *S) TestRecordingFsOpenFileReadAndWriteENOENT(c *check.C) {
	fs := RecordingFs{}
	f, err := fs.OpenFile("/my/file", syscall.O_RDWR, 0600)
	c.Assert(f, check.IsNil)
	c.Assert(err, check.Equals, syscall.ENOENT)
}

func (s *S) TestRecordingFsKeepFileInstances(c *check.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.Create("/my/file")
	c.Assert(err, check.IsNil)
	f.Write([]byte("hi"))
	f, err = fs.Open("/my/file")
	c.Assert(err, check.IsNil)
	buf := make([]byte, 2)
	n, err := f.Read(buf)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2)
	c.Assert(string(buf), check.Equals, "hi")
	// Opening again should read seek to position 0 in the reader
	f, _ = fs.Open("/my/file")
	n, err = f.Read(buf)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 2)
	c.Assert(string(buf), check.Equals, "hi")
}

func (s *S) TestRecordingFsShouldKeepWrittenContent(c *check.C) {
	fs := RecordingFs{FileContent: "the content"}
	f, _ := fs.Open("/my/file")
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	f.Close()
	c.Assert(string(buf[:n]), check.Equals, "the content")
	f, _ = fs.Create("/my/file")
	f.Write([]byte("content the"))
	f.Close()
	f, _ = fs.Open("/my/file")
	n, _ = f.Read(buf)
	c.Assert(string(buf[:n]), check.Equals, "content the")
}

func (s *S) TestRecordingFsFailToOpenUnknownFilesWithoutContent(c *check.C) {
	fs := RecordingFs{}
	f, err := fs.Open("/my/file")
	c.Assert(f, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(os.IsNotExist(err), check.Equals, true)
}

func (s *S) TestRecordingFsRemove(c *check.C) {
	fs := RecordingFs{}
	err := fs.Remove("/my/file")
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("remove /my/file"), check.Equals, true)
}

func (s *S) TestRecordingFsRemoveDeletesState(c *check.C) {
	fs := RecordingFs{FileContent: "hi"}
	f, _ := fs.Open("/my/file")
	f.Write([]byte("ih"))
	fs.Remove("/my/file")
	f, _ = fs.Open("/my/file")
	buf := make([]byte, 2)
	f.Read(buf)
	c.Assert(string(buf), check.Equals, "hi")
}

func (s *S) TestRecordingFsRemoveAll(c *check.C) {
	fs := RecordingFs{}
	err := fs.RemoveAll("/my/dir")
	c.Assert(err, check.IsNil)
	c.Assert(fs.HasAction("removeall /my/dir"), check.Equals, true)
}

func (s *S) TestRecordingFsRemoveAllDeletesState(c *check.C) {
	fs := RecordingFs{FileContent: "hi"}
	f, _ := fs.Open("/my/file")
	f.Write([]byte("ih"))
	fs.RemoveAll("/my/file")
	f, _ = fs.Open("/my/file")
	buf := make([]byte, 2)
	f.Read(buf)
	c.Assert(string(buf), check.Equals, "hi")
}

func (s *S) TestRecordingFsRename(c *check.C) {
	fs := RecordingFs{}
	f, _ := fs.Create("/my/file")
	f.Write([]byte("hello, hello!"))
	f.Close()
	err := fs.Rename("/my/file", "/your/file")
	c.Assert(err, check.IsNil)
	_, err = fs.Open("/my/file")
	c.Assert(err, check.NotNil)
	f, err = fs.Open("/your/file")
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, _ := ioutil.ReadAll(f)
	c.Assert(string(b), check.Equals, "hello, hello!")
	c.Assert(fs.HasAction("rename /my/file /your/file"), check.Equals, true)
}

func (s *S) TestRecordingFsCold(c *check.C) {
	fs := RecordingFs{}
	err := fs.Rename("/my/file", "/your/file")
	c.Assert(err, check.IsNil)
}

func (s *S) TestRecordingFsStat(c *check.C) {
	fs := RecordingFs{}
	fi, err := fs.Stat("/my/file")
	c.Assert(err, check.IsNil)
	c.Assert(fi, check.IsNil)
	c.Assert(fs.HasAction("stat /my/file"), check.Equals, true)
}

func (s *S) TestFileNotFoundFsPointerImplementsFsInterface(c *check.C) {
	var _ fs.Fs = &FileNotFoundFs{}
}

func (s *S) TestFileNotFoundFsOpen(c *check.C) {
	fs := FileNotFoundFs{}
	f, err := fs.Open("/my/file")
	c.Assert(f, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, check.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, check.Equals, "/my/file")
	c.Assert(fs.HasAction("open /my/file"), check.Equals, true)
}

func (s *S) TestFileNotFoundFsRemove(c *check.C) {
	fs := FileNotFoundFs{}
	err := fs.Remove("/my/file")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, check.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, check.Equals, "/my/file")
	c.Assert(fs.HasAction("remove /my/file"), check.Equals, true)
}

func (s *S) TestFileNotFoundFsOpenFile(c *check.C) {
	fs := FileNotFoundFs{}
	f, err := fs.OpenFile("/my/file", 0, 0600)
	c.Assert(f, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, check.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, check.Equals, "/my/file")
	c.Assert(fs.HasAction("open /my/file"), check.Equals, true)
}

func (s *S) TestFileNotFoundFsRemoveAll(c *check.C) {
	fs := FileNotFoundFs{}
	err := fs.RemoveAll("/my/file")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, check.DeepEquals, syscall.ENOENT)
	c.Assert(err.(*os.PathError).Path, check.Equals, "/my/file")
	c.Assert(fs.HasAction("removeall /my/file"), check.Equals, true)
}

func (s *S) TestFailureFsOpen(c *check.C) {
	origErr := errors.New("something went wrong")
	fs := FailureFs{Err: origErr}
	file, gotErr := fs.Open("/wat")
	c.Assert(file, check.IsNil)
	c.Assert(gotErr, check.Equals, origErr)
}
