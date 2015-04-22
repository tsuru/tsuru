// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"gopkg.in/check.v1"
	"net"
	"net/http"
	"time"
)

type fakeConn struct {
	closeCalls int
}

func (c *fakeConn) Read(b []byte) (n int, err error) {
	return len(b), nil
}
func (c *fakeConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}
func (c *fakeConn) Close() error {
	c.closeCalls++
	return nil
}
func (c *fakeConn) LocalAddr() net.Addr {
	return &net.IPAddr{}
}
func (c *fakeConn) RemoteAddr() net.Addr {
	return &net.IPAddr{}
}
func (c *fakeConn) SetDeadline(t time.Time) error {
	return nil
}
func (c *fakeConn) SetReadDeadline(t time.Time) error {
	return nil
}
func (c *fakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *S) TestIdleTrackerWithIdle(c *check.C) {
	tracker := newIdleTracker()
	conn := &fakeConn{}
	tracker.trackConn(conn, http.StateIdle)
	tracker.Shutdown()
	c.Assert(conn.closeCalls, check.Equals, 1)
}

func (s *S) TestIdleTrackerWithIdleAndClose(c *check.C) {
	tracker := newIdleTracker()
	conn := &fakeConn{}
	tracker.trackConn(conn, http.StateIdle)
	tracker.trackConn(conn, http.StateClosed)
	tracker.Shutdown()
	c.Assert(conn.closeCalls, check.Equals, 0)
	conn = &fakeConn{}
	tracker.trackConn(conn, http.StateIdle)
	tracker.trackConn(conn, http.StateHijacked)
	tracker.Shutdown()
	c.Assert(conn.closeCalls, check.Equals, 0)
}
