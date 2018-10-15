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
	openConns = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsuru_storage_connections_open",
		Help: "The current number of open connections to the storage.",
	})

	opBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_storage_operation_bytes_total",
		Help: "The total number of bytes used by storage operations.",
	}, []string{"op"})

	opErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tsuru_storage_operation_errors_total",
		Help: "The total number of errors during storage operations.",
	}, []string{"op"})

	latencies = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "tsuru_storage_duration_seconds",
		Help: "The storage operations latency distributions.",
	})
)

func init() {
	prometheus.MustRegister(openConns)
	prometheus.MustRegister(opBytes)
	prometheus.MustRegister(opErrors)
	prometheus.MustRegister(latencies)
}

func instrumentedDialServer(timeout time.Duration) func(*mgo.ServerAddr) (net.Conn, error) {
	return func(addr *mgo.ServerAddr) (net.Conn, error) {
		t0 := time.Now()
		defer func() {
			latencies.Observe(time.Since(t0).Seconds())
		}()
		conn, err := net.DialTimeout("tcp", addr.TCPAddr().String(), timeout)
		if err != nil {
			opErrors.WithLabelValues(opDial).Inc()
			return nil, err
		}
		if tcpconn, ok := conn.(*net.TCPConn); ok {
			tcpconn.SetKeepAlive(true)
			openConns.Inc()
			return &instrumentedConn{tcpConn: tcpconn}, nil
		}
		panic("internal error: obtained TCP connection is not a *net.TCPConn!?")
	}
}

type instrumentedConn struct {
	tcpConn *net.TCPConn
}

func (c *instrumentedConn) Read(b []byte) (n int, err error) {
	t0 := time.Now()
	defer func() {
		opBytes.WithLabelValues(opRead).Add(float64(n))
		if err != nil {
			opErrors.WithLabelValues(opRead).Inc()
		}
		latencies.Observe(time.Since(t0).Seconds())
	}()
	return c.tcpConn.Read(b)
}

func (c *instrumentedConn) Write(b []byte) (n int, err error) {
	t0 := time.Now()
	defer func() {
		opBytes.WithLabelValues(opWrite).Add(float64(n))
		if err != nil {
			opErrors.WithLabelValues(opWrite).Inc()
		}
		latencies.Observe(time.Since(t0).Seconds())
	}()
	return c.tcpConn.Write(b)
}

func (c *instrumentedConn) Close() (err error) {
	defer func() {
		openConns.Dec()
		if err != nil {
			opErrors.WithLabelValues(opClose).Inc()
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
