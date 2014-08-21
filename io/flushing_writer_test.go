// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"

	"launchpad.net/gocheck"
)

func (s *S) TestFlushingWriter(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(recorder.Body.Bytes(), gocheck.DeepEquals, data)
	c.Assert(writer.wrote, gocheck.Equals, true)
}

func (s *S) TestFlushingWriterShouldReturnTheDataSize(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, gocheck.IsNil)
	c.Assert(n, gocheck.Equals, len(data))
}

func (s *S) TestFlushingWriterHeader(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), gocheck.Equals, "application/xml")
}

func (s *S) TestFlushingWriterWriteHeader(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, gocheck.Equals, expectedCode)
	c.Assert(writer.wrote, gocheck.Equals, true)
}

func (s *S) TestFlushingWriterWrote(c *gocheck.C) {
	writer := FlushingWriter{nil, false}
	c.Assert(writer.Wrote(), gocheck.Equals, false)
	writer.wrote = true
	c.Assert(writer.Wrote(), gocheck.Equals, true)
}

func (s *S) TestFlushingWriterHijack(c *gocheck.C) {
	var buf bytes.Buffer
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gocheck.IsNil)
	defer listener.Close()
	expectedConn, err := net.Dial("tcp", listener.Addr().String())
	c.Assert(err, gocheck.IsNil)
	recorder := hijacker{
		ResponseWriter: httptest.NewRecorder(),
		input:          &buf,
		conn:           expectedConn,
	}
	writer := FlushingWriter{&recorder, false}
	conn, rw, err := writer.Hijack()
	c.Assert(err, gocheck.IsNil)
	c.Assert(conn, gocheck.Equals, expectedConn)
	buf.Write([]byte("hello world"))
	b, err := ioutil.ReadAll(rw)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(b), gocheck.Equals, "hello world")
	rw.Write([]byte("hi, how are you?"))
	body := recorder.ResponseWriter.(*httptest.ResponseRecorder).Body.String()
	c.Assert(body, gocheck.Equals, "hi, how are you?")
}

func (s *S) TestFlushingWriterFailureToHijack(c *gocheck.C) {
	expectedErr := errors.New("failed to hijack, man")
	recorder := hijacker{err: expectedErr}
	writer := FlushingWriter{&recorder, false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, gocheck.IsNil)
	c.Assert(rw, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, expectedErr)
}

func (s *S) TestFlushingWriterHijackUnhijackable(c *gocheck.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{recorder, false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, gocheck.IsNil)
	c.Assert(rw, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "cannot hijack connection")
}

type hijacker struct {
	http.ResponseWriter
	input io.Reader
	conn  net.Conn
	err   error
}

func (h *hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.err != nil {
		return nil, nil, h.err
	}
	rw := bufio.ReadWriter{
		Reader: bufio.NewReader(h.input),
		Writer: bufio.NewWriterSize(h.ResponseWriter, 1),
	}
	return h.conn, &rw, nil
}
