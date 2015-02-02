// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/safe"
)

type FakeConn struct {
	Buf *safe.Buffer
}

func (c *FakeConn) Read(b []byte) (int, error) {
	if c.Buf != nil {
		return c.Buf.Read(b)
	}
	return 0, io.EOF
}

func (c *FakeConn) Write(b []byte) (int, error) {
	if c.Buf != nil {
		return c.Buf.Write(b)
	}
	return 0, io.ErrClosedPipe
}

func (c *FakeConn) Close() error {
	c.Buf = nil
	return nil
}

func (c *FakeConn) LocalAddr() net.Addr {
	return nil
}

func (c *FakeConn) RemoteAddr() net.Addr {
	return nil
}

func (c *FakeConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *FakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *FakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type Hijacker struct {
	http.ResponseWriter
	Conn net.Conn
	err  error
}

func (h *Hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.err != nil {
		return nil, nil, h.err
	}
	return h.Conn, nil, nil
}
