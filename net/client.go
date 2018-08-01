// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
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
			IdleConnTimeout:     15 * time.Second,
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

	Dial10Full60ClientWithPool, _ = makeTimeoutHTTPClient(10*time.Second, 1*time.Minute, 10, true)
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

func WithProxy(cli http.Client, proxyURL string) (*http.Client, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	newTransport := http.Transport{
		Proxy: http.ProxyURL(u),
	}
	baseTrans, _ := cli.Transport.(*http.Transport)
	if baseTrans != nil {
		newTransport.Dial = baseTrans.Dial
		newTransport.TLSHandshakeTimeout = baseTrans.TLSHandshakeTimeout
		newTransport.MaxIdleConnsPerHost = baseTrans.MaxIdleConnsPerHost
		newTransport.TLSClientConfig = baseTrans.TLSClientConfig
	}
	cli.Transport = &newTransport
	return &cli, nil
}
