// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	check "gopkg.in/check.v1"
)

func (s *S) TestFlushingWriter(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	data := []byte("ble")
	_, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, data)
	c.Assert(writer.wrote, check.Equals, true)
}

func (s *S) TestFlushingWriterShouldReturnTheDataSize(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	data := []byte("ble")
	n, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, len(data))
}

func (s *S) TestFlushingWriterHeader(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	writer.Header().Set("Content-Type", "application/xml")
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/xml")
}

func (s *S) TestFlushingWriterWriteHeader(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	expectedCode := 333
	writer.WriteHeader(expectedCode)
	c.Assert(recorder.Code, check.Equals, expectedCode)
	c.Assert(writer.wrote, check.Equals, true)
}

func (s *S) TestFlushingWriterWrote(c *check.C) {
	writer := FlushingWriter{WriterFlusher: nil}
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
	writer := FlushingWriter{WriterFlusher: &recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(err, check.IsNil)
	c.Assert(conn, check.Equals, expectedConn)
	buf.Write([]byte("hello world"))
	b, err := io.ReadAll(rw)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, "hello world")
	rw.Write([]byte("hi, how are you?"))
	body := recorder.ResponseWriter.(*httptest.ResponseRecorder).Body.String()
	c.Assert(body, check.Equals, "hi, how are you?")
}

func (s *S) TestFlushingWriterFailureToHijack(c *check.C) {
	expectedErr := errors.New("failed to hijack, man")
	recorder := hijacker{err: expectedErr}
	writer := FlushingWriter{WriterFlusher: &recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, check.IsNil)
	c.Assert(rw, check.IsNil)
	c.Assert(err, check.Equals, expectedErr)
}

func (s *S) TestFlushingWriterHijackUnhijackable(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	conn, rw, err := writer.Hijack()
	c.Assert(conn, check.IsNil)
	c.Assert(rw, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "cannot hijack connection")
}

func (s *S) TestFlushingWriterOfFlushingWriter(c *check.C) {
	recorder := httptest.NewRecorder()
	writer := FlushingWriter{WriterFlusher: recorder, wrote: false}
	writer2 := FlushingWriter{WriterFlusher: &writer}
	data := []byte("ble")
	_, err := writer2.Write(data)
	c.Assert(err, check.IsNil)
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, data)
	c.Assert(writer.wrote, check.Equals, true)
	c.Assert(writer2.wrote, check.Equals, true)
}

type blockWriterFlusher struct {
	*httptest.ResponseRecorder
	flushCh chan struct{}
}

func (w *blockWriterFlusher) Flush() {
	w.flushCh <- struct{}{}
}

func (s *S) TestFlushingWriterCustomLatency(c *check.C) {
	recorder := &blockWriterFlusher{
		ResponseRecorder: httptest.NewRecorder(),
		flushCh:          make(chan struct{}),
	}
	latency := 100 * time.Millisecond
	writer := FlushingWriter{WriterFlusher: recorder, MaxLatency: latency}
	data := []byte("ble")
	t0 := time.Now()
	_, err := writer.Write(data)
	c.Assert(err, check.IsNil)
	select {
	case <-recorder.flushCh:
		c.Assert(time.Since(t0) >= latency, check.Equals, true)
	case <-time.After(5 * time.Second):
		c.Fatal("timeout waiting flush call")
	}
	c.Assert(recorder.Body.Bytes(), check.DeepEquals, data)
	c.Assert(writer.wrote, check.Equals, true)
}

type hijacker struct {
	http.ResponseWriter
	input io.Reader
	conn  net.Conn
	err   error
}

func (h *hijacker) Flush() {}

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

func (s *S) TestFlushingWriterFlushAfterWrite(c *check.C) {
	wg := sync.WaitGroup{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(WriterFlusher)
		c.Assert(ok, check.Equals, true)
		fw := FlushingWriter{WriterFlusher: flusher}
		defer fw.Close()
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fw.Write([]byte("a"))
			}()
		}
	}))
	defer srv.Close()
	rsp, err := http.Get(srv.URL)
	c.Assert(err, check.IsNil)
	defer rsp.Body.Close()
	_, err = io.ReadAll(rsp.Body)
	c.Assert(err, check.IsNil)
	wg.Wait()
}
