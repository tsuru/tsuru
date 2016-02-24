// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"net"
	"net/http"
	"time"
)

func makeTimeoutHTTPClient(dialTimeout time.Duration, fullTimeout time.Duration, maxIdle int) (*http.Client, *net.Dialer) {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}
	client := &http.Client{
		Transport: &http.Transport{
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: dialTimeout,
			MaxIdleConnsPerHost: maxIdle,
		},
		Timeout: fullTimeout,
	}
	return client, dialer
}

var (
	Dial5Full60Client, Dial5Dialer   = makeTimeoutHTTPClient(5*time.Second, 1*time.Minute, 5)
	Dial5Full300Client, _            = makeTimeoutHTTPClient(5*time.Second, 5*time.Minute, 5)
	Dial5FullUnlimitedClient, _      = makeTimeoutHTTPClient(5*time.Second, 0, 5)
	Dial5Full300ClientNoKeepAlive, _ = makeTimeoutHTTPClient(5*time.Second, 5*time.Minute, -1)
	Dial5Full60ClientNoKeepAlive, _  = makeTimeoutHTTPClient(5*time.Second, 1*time.Minute, -1)
)
