// Copyright 2015 tsuru authors. All rights reserved.
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

	"gopkg.in/check.v1"
)

func (s *S) TestFlushingWriter(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder, wrote: false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, data)
	c.Assert(writer.wrote, check.Equals, true)
}

func (s *S) TestFlushingWriterShouldReturnTheDataSize(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder, wrote: false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(data))
}

func (s *S) TestFlushingWriterHeader(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder, wrote: false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/xml")
}

func (s *S) TestFlushingWriterWriteHeader(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder, wrote: false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, check.Equals, expectedCode)
	c.Assert(writer.wrote, check.Equals, true)
}

func (s *S) TestFlushingWriterWrote(c *check.C) {
	writer := FlushingWriter{ResponseWriter: nil, wrote: false}
	c.Assert(writer.Wrote(), check.Equals, false)
	writer.wrote = true
	c.Assert(writer.Wrote(), check.Equals, true)
}

func (s *S) TestFlushingWriterHijack(c *check.C) {
	var buf bytes.Buffer
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer listener.Close()
	expectedConn, err := net.Dial("tcp", listener.Addr().String())
	c.Assert(err, check.IsNil)
	recorder := hijacker{
		ResponseWriter: httptest.NewRecorder(),
		input:          &buf,
		conn:           expectedConn,
	}
	writer := FlushingWriter{ResponseWriter: &recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(err, check.IsNil)
	c.Assert(conn, check.Equals, expectedConn)
	buf.Write([]byte("hello world"))
	b, err := ioutil.ReadAll(rw)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, "hello world")
	rw.Write([]byte("hi, how are you?"))
	body := recorder.ResponseWriter.(*httptest.ResponseRecorder).Body.String()
	c.Assert(body, check.Equals, "hi, how are you?")
}

func (s *S) TestFlushingWriterFailureToHijack(c *check.C) {
	expectedErr := errors.New("failed to hijack, man")
	recorder := hijacker{err: expectedErr}
	writer := FlushingWriter{ResponseWriter: &recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, check.IsNil)
	c.Assert(rw, check.IsNil)
	c.Assert(err, check.Equals, expectedErr)
}

func (s *S) TestFlushingWriterHijackUnhijackable(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, check.IsNil)
	c.Assert(rw, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cannot hijack connection")
}

func (s *S) TestFlushingWriterOfFlushingWriter(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{ResponseWriter: recorder}
	writer2 := FlushingWriter{ResponseWriter: &writer}
	data := []byte("ble")
	_, err := writer2.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, data)
	c.Assert(writer.wrote, check.Equals, true)
	c.Assert(writer2.wrote, check.Equals, true)
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
