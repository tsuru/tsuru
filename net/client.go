// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

func makeTimeoutHTTPClient(dialTimeout time.Duration, fullTimeout time.Duration, maxIdle int, followRedirects bool) (*http.Client, *net.Dialer) {
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
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client, dialer
}

const (
	StreamInactivityTimeout = time.Minute
)

var (
	Dial5Full300Client, Dial5Dialer                = makeTimeoutHTTPClient(5*time.Second, 5*time.Minute, 5, true)
	Dial5FullUnlimitedClient, _                    = makeTimeoutHTTPClient(5*time.Second, 0, 5, true)
	Dial5Full300ClientNoKeepAlive, _               = makeTimeoutHTTPClient(5*time.Second, 5*time.Minute, -1, true)
	Dial5Full60ClientNoKeepAlive, _                = makeTimeoutHTTPClient(5*time.Second, 1*time.Minute, -1, true)
	Dial5Full60ClientNoKeepAliveNoRedirect, _      = makeTimeoutHTTPClient(5*time.Second, 1*time.Minute, -1, false)
	Dial5Full60ClientNoKeepAliveNoRedirectInsecure = insecure(*Dial5Full60ClientNoKeepAliveNoRedirect)
	Dial5Full60ClientNoKeepAliveInsecure           = insecure(*Dial5Full60ClientNoKeepAlive)
)

func insecure(client http.Client) http.Client {
	tlsConfig := client.Transport.(*http.Transport).TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}
	tlsConfig.InsecureSkipVerify = true
	client.Transport.(*http.Transport).TLSClientConfig = tlsConfig
	return client
}
