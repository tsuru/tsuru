package multibuf

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type BufferSuite struct{}

var _ = Suite(&BufferSuite{})

func createReaderOfSize(size int64) (reader io.Reader, hash string) {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}

	b := make([]byte, int(size))

	_, err = io.ReadFull(f, b)

	if err != nil {
		panic(err)
	}

	h := md5.New()
	h.Write(b)
	return bytes.NewReader(b), hex.EncodeToString(h.Sum(nil))
}

func hashOfReader(r io.Reader) string {
	h := md5.New()
	tr := io.TeeReader(r, h)
	_, _ = io.Copy(ioutil.Discard, tr)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *BufferSuite) TestSmallBuffer(c *C) {
	r, hash := createReaderOfSize(1)
	bb, err := New(r)
	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)
	bb.Close()
}

func (s *BufferSuite) TestBigBuffer(c *C) {
	r, hash := createReaderOfSize(13631488)
	bb, err := New(r)
	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)
}

func (s *BufferSuite) TestSeek(c *C) {
	tlen := int64(1057576)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r)

	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)
	l, err := bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)

	bb.Seek(0, 0)
	c.Assert(hashOfReader(bb), Equals, hash)
	l, err = bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)
}

func (s *BufferSuite) TestSeekWithFile(c *C) {
	tlen := int64(DefaultMemBytes)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r, MemBytes(1))

	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)
	l, err := bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)

	bb.Seek(0, 0)
	c.Assert(hashOfReader(bb), Equals, hash)
	l, err = bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)
}

func (s *BufferSuite) TestSeekFirst(c *C) {
	tlen := int64(1057576)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r)

	l, err := bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)

	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)

	bb.Seek(0, 0)

	c.Assert(hashOfReader(bb), Equals, hash)
	l, err = bb.Size()
	c.Assert(err, IsNil)
	c.Assert(l, Equals, tlen)
}

func (s *BufferSuite) TestLimitDoesNotExceed(c *C) {
	requestSize := int64(1057576)
	r, hash := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(1024), MaxBytes(requestSize+1))
	c.Assert(err, IsNil)
	c.Assert(hashOfReader(bb), Equals, hash)
	size, err := bb.Size()
	c.Assert(err, IsNil)
	c.Assert(size, Equals, requestSize)
	bb.Close()
}

func (s *BufferSuite) TestLimitExceeds(c *C) {
	requestSize := int64(1057576)
	r, _ := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(1024), MaxBytes(requestSize-1))
	c.Assert(err, FitsTypeOf, &MaxSizeReachedError{})
	c.Assert(bb, IsNil)
}

func (s *BufferSuite) TestLimitExceedsMemBytes(c *C) {
	requestSize := int64(1057576)
	r, _ := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(requestSize+1), MaxBytes(requestSize-1))
	c.Assert(err, FitsTypeOf, &MaxSizeReachedError{})
	c.Assert(bb, IsNil)
}

func (s *BufferSuite) TestWriteToBigBuffer(c *C) {
	l := int64(13631488)
	r, hash := createReaderOfSize(l)
	bb, err := New(r)
	c.Assert(err, IsNil)

	other := &bytes.Buffer{}
	wrote, err := bb.WriteTo(other)
	c.Assert(err, IsNil)
	c.Assert(wrote, Equals, l)
	c.Assert(hashOfReader(other), Equals, hash)
}

func (s *BufferSuite) TestWriteToSmallBuffer(c *C) {
	l := int64(1)
	r, hash := createReaderOfSize(l)
	bb, err := New(r)
	c.Assert(err, IsNil)

	other := &bytes.Buffer{}
	wrote, err := bb.WriteTo(other)
	c.Assert(err, IsNil)
	c.Assert(wrote, Equals, l)
	c.Assert(hashOfReader(other), Equals, hash)
}

func (s *BufferSuite) TestWriterOnceSmallBuffer(c *C) {
	r, hash := createReaderOfSize(1)

	w, err := NewWriterOnce()
	c.Assert(err, IsNil)

	total, err := io.Copy(w, r)
	c.Assert(err, Equals, nil)
	c.Assert(total, Equals, int64(1))

	bb, err := w.Reader()
	c.Assert(err, IsNil)

	c.Assert(hashOfReader(bb), Equals, hash)
	bb.Close()
}

func (s *BufferSuite) TestWriterOnceBigBuffer(c *C) {
	size := int64(13631488)
	r, hash := createReaderOfSize(size)

	w, err := NewWriterOnce()
	c.Assert(err, IsNil)

	total, err := io.Copy(w, r)
	c.Assert(err, Equals, nil)
	c.Assert(total, Equals, size)

	bb, err := w.Reader()
	c.Assert(err, IsNil)

	c.Assert(hashOfReader(bb), Equals, hash)
	bb.Close()
}

func (s *BufferSuite) TestWriterOncePartialWrites(c *C) {
	size := int64(13631488)
	r, hash := createReaderOfSize(size)

	w, err := NewWriterOnce()
	c.Assert(err, IsNil)

	total, err := io.CopyN(w, r, DefaultMemBytes+1)
	c.Assert(err, Equals, nil)
	c.Assert(total, Equals, int64(DefaultMemBytes+1))

	remained := size - DefaultMemBytes - 1
	total, err = io.CopyN(w, r, remained)
	c.Assert(err, Equals, nil)
	c.Assert(int64(total), Equals, remained)

	bb, err := w.Reader()
	c.Assert(err, IsNil)
	c.Assert(w.(*writerOnce).mem, IsNil)
	c.Assert(w.(*writerOnce).file, IsNil)

	c.Assert(hashOfReader(bb), Equals, hash)
	bb.Close()
}

func (s *BufferSuite) TestWriterOnceMaxSizeExceeded(c *C) {
	size := int64(1000)
	r, _ := createReaderOfSize(size)

	w, err := NewWriterOnce(MemBytes(10), MaxBytes(100))
	c.Assert(err, IsNil)

	_, err = io.Copy(w, r)
	c.Assert(err, NotNil)
	c.Assert(w.Close(), IsNil)
}

func (s *BufferSuite) TestWriterReaderCalled(c *C) {
	size := int64(1000)
	r, hash := createReaderOfSize(size)

	w, err := NewWriterOnce()
	c.Assert(err, IsNil)

	_, err = io.Copy(w, r)
	c.Assert(err, IsNil)
	c.Assert(w.Close(), IsNil)

	bb, err := w.Reader()
	c.Assert(err, IsNil)

	c.Assert(hashOfReader(bb), Equals, hash)

	// Subsequent calls to write and get reader will fail
	_, err = w.Reader()
	c.Assert(err, NotNil)

	_, err = w.Write([]byte{1})
	c.Assert(err, NotNil)
}

func (s *BufferSuite) TestWriterNoData(c *C) {
	w, err := NewWriterOnce()
	c.Assert(err, IsNil)

	_, err = w.Reader()
	c.Assert(err, NotNil)
}
