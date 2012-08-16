package testing

import (
	"github.com/timeredbull/tsuru/fs"
	. "launchpad.net/gocheck"
	"os"
	"syscall"
	"testing"
)

type S struct{}

var _ = Suite(&S{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *S) TestFakeFilePointerShouldImplementFileInterface(c *C) {
	var file fs.File
	c.Assert(&FakeFile{}, Implements, &file)
}

func (s *S) TestFakeFileCloseJustReturnNil(c *C) {
	f := &FakeFile{content: "doesn't matter"}
	err := f.Close()
	c.Assert(err, IsNil)
}

func (s *S) TestFakeFileRead(c *C) {
	content := "all I am"
	f := &FakeFile{content: content}
	buf := make([]byte, 20)
	n, err := f.Read(buf)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len(content))
	c.Assert(string(buf[:n]), Equals, content)
}

func (s *S) TestFakeFileReadAt(c *C) {
	content := "invisible cage"
	f := &FakeFile{content: content}
	buf := make([]byte, 4)
	n, err := f.ReadAt(buf, int64(len(content)-len(buf)))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 4)
	c.Assert(string(buf), Equals, "cage")
}

func (s *S) TestFakeFileSeek(c *C) {
	content := "fragile equality"
	f := &FakeFile{content: content}
	n, err := f.Seek(8, 0)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, int64(8))
	buf := make([]byte, 5)
	read, err := f.Read(buf)
	c.Assert(err, IsNil)
	c.Assert(read, Equals, 5)
	c.Assert(string(buf), Equals, "equal")
}

func (s *S) TestFakeFileStat(c *C) {
	var empty os.FileInfo
	f := &FakeFile{content: "doesn't matter"}
	fi, err := f.Stat()
	c.Assert(err, IsNil)
	c.Assert(fi, DeepEquals, empty)
}

func (s *S) TestFakeFileWrite(c *C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Write([]byte("break"))
	c.Assert(err, IsNil)
	c.Assert(n, Equals, len("break"))
	c.Assert(f.content, Equals, "break")
}

func (s *S) TestFakeFileWriteFromPosition(c *C) {
	content := "Guardian"
	f := &FakeFile{content: content}
	n, err := f.Seek(5, 0)
	c.Assert(err, IsNil)
	c.Assert(n, Equals, int64(5))
	written, err := f.Write([]byte("break"))
	c.Assert(err, IsNil)
	c.Assert(written, Equals, len("break"))
	c.Assert(f.content, Equals, "Guardbreak")
}

func (s *S) TestRecordingFsPointerShouldImplementFsInterface(c *C) {
	var fs fs.Fs
	c.Assert(&RecordingFs{}, Implements, &fs)
}

func (s *S) TestRecordingFsHasAction(c *C) {
	fs := RecordingFs{actions: []string{"torn", "shade of my soul"}}
	c.Assert(fs.HasAction("torn"), Equals, true)
	c.Assert(fs.HasAction("shade of my soul"), Equals, true)
	c.Assert(fs.HasAction("meaningles world"), Equals, false)
}

func (s *S) TestRecordingFsCreate(c *C) {
	fs := RecordingFs{}
	f, err := fs.Create("/my/file.txt")
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("create /my/file.txt"), Equals, true)
	c.Assert(f, FitsTypeOf, &FakeFile{})
}

func (s *S) TestRecordingFsMkdir(c *C) {
	fs := RecordingFs{}
	err := fs.Mkdir("/my/dir", 0777)
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("mkdir /my/dir with mode 0777"), Equals, true)
}

func (s *S) TestRecordingFsMkdirAll(c *C) {
	fs := RecordingFs{}
	err := fs.MkdirAll("/my/dir/with/subdir", 0777)
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("mkdirall /my/dir/with/subdir with mode 0777"), Equals, true)
}

func (s *S) TestRecordingFsOpen(c *C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.Open("/my/file")
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("open /my/file"), Equals, true)
	c.Assert(f, FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, Equals, fs.FileContent)
}

func (s *S) TestRecordingFsOpenFile(c *C) {
	fs := RecordingFs{FileContent: "the content"}
	f, err := fs.OpenFile("/my/file", 0, 0600)
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("openfile /my/file with mode 0600"), Equals, true)
	c.Assert(f, FitsTypeOf, &FakeFile{})
	c.Assert(f.(*FakeFile).content, Equals, fs.FileContent)
}

func (s *S) TestRecordingFsRemove(c *C) {
	fs := RecordingFs{}
	err := fs.Remove("/my/file")
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("remove /my/file"), Equals, true)
}

func (s *S) TestRecordingFsRemoveAll(c *C) {
	fs := RecordingFs{}
	err := fs.RemoveAll("/my/dir")
	c.Assert(err, IsNil)
	c.Assert(fs.HasAction("removeall /my/dir"), Equals, true)
}

func (s *S) TestRecordingFsStat(c *C) {
	fs := RecordingFs{}
	fi, err := fs.Stat("/my/file")
	c.Assert(err, IsNil)
	c.Assert(fi, IsNil)
	c.Assert(fs.HasAction("stat /my/file"), Equals, true)
}

func (s *S) TestFailureFsPointerImplementsFsInterface(c *C) {
	var fs fs.Fs
	c.Assert(&FailureFs{}, Implements, &fs)
}

func (s *S) TestFailureFsOpen(c *C) {
	fs := FailureFs{}
	f, err := fs.Open("/my/file")
	c.Assert(f, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err, FitsTypeOf, &os.PathError{})
	c.Assert(err.(*os.PathError).Err, DeepEquals, syscall.ENOENT)
	c.Assert(fs.HasAction("open /my/file"), Equals, true)
}
