// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"net"
	"time"

	mgo "github.com/globalsign/mgo"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	opRead  = "read"
	opWrite = "write"
	opDial  = "dial"
	opClose = "close"
)

var (
	openConns = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tsuru_storage_connections_open",
		Help: "The current number of open connections to the storage.",
	}, []string{"db"})

	opBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_storage_operation_bytes_total",
		Help: "The total number of bytes used by storage operations.",
	}, []string{"db", "op"})

	opErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_storage_operation_errors_total",
		Help: "The total number of errors during storage operations.",
	}, []string{"db", "op"})

	ioBlock = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_storage_io_seconds_total",
		Help: "The total time blocked in i/o operations to the storage.",
	}, []string{"db", "op"})

	latencies = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tsuru_storage_duration_seconds",
		Help:    "The storage operations latency distributions.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"db", "op"})
)

func init() {
	prometheus.MustRegister(openConns)
	prometheus.MustRegister(opBytes)
	prometheus.MustRegister(opErrors)
	prometheus.MustRegister(ioBlock)
	prometheus.MustRegister(latencies)
}

func instrumentedDialServer(timeout time.Duration) func(*mgo.ServerAddr) (net.Conn, error) {
	return func(addr *mgo.ServerAddr) (net.Conn, error) {
		t0 := time.Now()
		defer func() {
			deltaT := time.Since(t0).Seconds()
			latencies.WithLabelValues(addr.String(), opDial).Observe(deltaT)
			ioBlock.WithLabelValues(addr.String(), opDial).Add(deltaT)
		}()
		conn, err := net.DialTimeout("tcp", addr.TCPAddr().String(), timeout)
		if err != nil {
			opErrors.WithLabelValues(addr.String(), opDial).Inc()
			return nil, err
		}
		if tcpconn, ok := conn.(*net.TCPConn); ok {
			tcpconn.SetKeepAlive(true)
			openConns.WithLabelValues(addr.String()).Inc()
			return &instrumentedConn{tcpConn: tcpconn, addr: addr.String()}, nil
		}
		panic("internal error: obtained TCP connection is not a *net.TCPConn!?")
	}
}

type instrumentedConn struct {
	tcpConn *net.TCPConn
	addr    string
}

func (c *instrumentedConn) Read(b []byte) (n int, err error) {
	t0 := time.Now()
	defer func() {
		deltaT := time.Since(t0).Seconds()
		latencies.WithLabelValues(c.addr, opRead).Observe(deltaT)
		ioBlock.WithLabelValues(c.addr, opRead).Add(deltaT)
		opBytes.WithLabelValues(c.addr, opRead).Add(float64(n))
		if err != nil {
			opErrors.WithLabelValues(c.addr, opRead).Inc()
		}
	}()
	return c.tcpConn.Read(b)
}

func (c *instrumentedConn) Write(b []byte) (n int, err error) {
	t0 := time.Now()
	defer func() {
		deltaT := time.Since(t0).Seconds()
		latencies.WithLabelValues(c.addr, opWrite).Observe(deltaT)
		ioBlock.WithLabelValues(c.addr, opWrite).Add(deltaT)
		opBytes.WithLabelValues(c.addr, opWrite).Add(float64(n))
		if err != nil {
			opErrors.WithLabelValues(c.addr, opWrite).Inc()
		}
	}()
	return c.tcpConn.Write(b)
}

func (c *instrumentedConn) Close() (err error) {
	t0 := time.Now()
	defer func() {
		deltaT := time.Since(t0).Seconds()
		latencies.WithLabelValues(c.addr, opClose).Observe(deltaT)
		ioBlock.WithLabelValues(c.addr, opClose).Add(deltaT)
		openConns.WithLabelValues(c.addr).Dec()
		if err != nil {
			opErrors.WithLabelValues(c.addr, opClose).Inc()
		}
	}()
	return c.tcpConn.Close()
}

func (c *instrumentedConn) LocalAddr() net.Addr {
	return c.tcpConn.LocalAddr()
}

func (c *instrumentedConn) RemoteAddr() net.Addr {
	return c.tcpConn.RemoteAddr()
}

func (c *instrumentedConn) SetDeadline(t time.Time) error {
	return c.tcpConn.SetDeadline(t)
}

func (c *instrumentedConn) SetReadDeadline(t time.Time) error {
	return c.tcpConn.SetReadDeadline(t)
}

func (c *instrumentedConn) SetWriteDeadline(t time.Time) error {
	return c.tcpConn.SetWriteDeadline(t)
}
