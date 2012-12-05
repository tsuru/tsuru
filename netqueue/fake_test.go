// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netqueue

import (
	"errors"
	"net"
	"sync/atomic"
	"time"
)

// Fake implementation of net.Conn.
type FakeConn struct {
	buf    *SafeBuffer
	closed int32
	laddr  net.Addr
	raddr  net.Addr
}

func NewFakeConn(laddr, raddr string) *FakeConn {
	localTcpAddr, err := net.ResolveTCPAddr("tcp", laddr)
	if err != nil {
		panic("Could not resolve local address: " + err.Error())
	}
	remoteTcpAddr, err := net.ResolveTCPAddr("tcp", raddr)
	if err != nil {
		panic("Could not resolve remote address: " + err.Error())
	}
	return &FakeConn{
		buf:   new(SafeBuffer),
		laddr: localTcpAddr,
		raddr: remoteTcpAddr,
	}
}

func (conn *FakeConn) Read(b []byte) (int, error) {
	if atomic.LoadInt32(&conn.closed) == 1 {
		return 0, errors.New("Closed connection.")
	}
	return conn.buf.Read(b)
}

func (conn *FakeConn) Write(b []byte) (int, error) {
	if atomic.LoadInt32(&conn.closed) == 1 {
		return 0, errors.New("Closed connection.")
	}
	return conn.buf.Write(b)
}

func (conn *FakeConn) Close() error {
	if atomic.LoadInt32(&conn.closed) == 1 {
		return errors.New("Connection already closed.")
	}
	conn.buf.Lock()
	defer conn.buf.Unlock()
	atomic.StoreInt32(&conn.closed, 1)
	return nil
}

func (conn *FakeConn) LocalAddr() net.Addr {
	return conn.laddr
}

func (conn *FakeConn) RemoteAddr() net.Addr {
	return conn.raddr
}

func (conn *FakeConn) SetDeadline(t time.Time) error {
	return nil
}

func (conn *FakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (conn *FakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Fake implementation of net.Listener.
type FakeListener struct {
	actions []string
	closed  int32
	laddr   net.Addr
}

func NewFakeListener(addr string) *FakeListener {
	laddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic("Could not resolve addr: " + err.Error())
	}
	return &FakeListener{
		laddr: laddr,
	}
}

func (f *FakeListener) Accept() (net.Conn, error) {
	if atomic.LoadInt32(&f.closed) == 1 {
		return nil, errors.New("Closed listener.")
	}
	f.actions = append(f.actions, "accept")
	return NewFakeConn(f.laddr.String(), "10.10.10.10:43023"), nil
}

func (f *FakeListener) Close() error {
	if atomic.LoadInt32(&f.closed) == 1 {
		return errors.New("Listener already closed.")
	}
	atomic.StoreInt32(&f.closed, 1)
	return nil
}

func (f *FakeListener) Addr() net.Addr {
	return f.laddr
}
