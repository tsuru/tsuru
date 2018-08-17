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
			DialContext:         dialer.DialContext,
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
	Dial15Full300Client, _                          = makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, 5, true)
	Dial15FullUnlimitedClient, _                    = makeTimeoutHTTPClient(15*time.Second, 0, 5, true)
	Dial15Full300ClientNoKeepAlive, _               = makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, -1, true)
	Dial15Full60ClientNoKeepAlive, _                = makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, true)
	Dial15Full60ClientNoKeepAliveNoRedirect, _      = makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, false)
	Dial15Full60ClientNoKeepAliveNoRedirectInsecure = insecure(*Dial15Full60ClientNoKeepAliveNoRedirect)
	Dial15Full60ClientNoKeepAliveInsecure           = insecure(*Dial15Full60ClientNoKeepAlive)

	Dial15Full60ClientWithPool, _  = makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, 10, true)
	Dial15Full300ClientWithPool, _ = makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, 10, true)
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
		newTransport.DialContext = baseTrans.DialContext
		newTransport.TLSHandshakeTimeout = baseTrans.TLSHandshakeTimeout
		newTransport.MaxIdleConnsPerHost = baseTrans.MaxIdleConnsPerHost
		newTransport.TLSClientConfig = baseTrans.TLSClientConfig
	}
	cli.Transport = &newTransport
	return &cli, nil
}
