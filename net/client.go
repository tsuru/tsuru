// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/tsuru/config"
)

func makeTimeoutHTTPClient(dialTimeout time.Duration, fullTimeout time.Duration, maxIdle int, followRedirects bool) *http.Client {
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
	return client
}

const (
	StreamInactivityTimeout = time.Minute
)

var (
	Dial15Full300Client                             = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, 5, true))
	Dial15FullUnlimitedClient                       = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 0, 5, true))
	Dial15Full300ClientNoKeepAlive                  = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, -1, true))
	Dial15Full60ClientNoKeepAlive                   = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, true))
	Dial15Full60ClientNoKeepAliveNoRedirect         = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, false))
	Dial15Full60ClientNoKeepAliveNoRedirectInsecure = insecure(withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, false)))
	Dial15Full60ClientNoKeepAliveInsecure           = insecure(withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, -1, true)))

	Dial15Full60ClientWithPool  = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 1*time.Minute, 10, true))
	Dial15Full300ClientWithPool = withOpenTelemetry(makeTimeoutHTTPClient(15*time.Second, 5*time.Minute, 10, true))
)

func insecure(client *http.Client) *http.Client {
	// Extract the base HTTP transport from the potentially wrapped otelhttp transport
	var httpTransport *http.Transport

	if baseRT, ok := client.Transport.(*http.Transport); ok {
		httpTransport = baseRT
	} else {
		// If it's wrapped, try to unwrap to configure TLS on base transport
		if baseTransport, ok := unwrapTransport(client.Transport); ok {
			httpTransport = baseTransport
		}
	}

	if httpTransport == nil {
		return client
	}

	tlsConfig := httpTransport.TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}
	tlsConfig.InsecureSkipVerify = true
	httpTransport.TLSClientConfig = tlsConfig
	return client
}

// unwrapTransport attempts to extract the base *http.Transport from wrapped transports
func unwrapTransport(rt http.RoundTripper) (*http.Transport, bool) {
	// For otelhttp and similar wrappers, try type assertion
	if t, ok := rt.(*http.Transport); ok {
		return t, true
	}
	// Add more unwrapping logic if needed for other transport wrappers
	return nil, false
}

func WithProxy(cli http.Client, proxyURL string) (*http.Client, error) {
	newTransport, err := proxyTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	baseTrans, _ := cli.Transport.(*http.Transport)
	if baseTrans != nil {
		newTransport.DialContext = baseTrans.DialContext
		newTransport.TLSHandshakeTimeout = baseTrans.TLSHandshakeTimeout
		newTransport.MaxIdleConnsPerHost = baseTrans.MaxIdleConnsPerHost
		newTransport.TLSClientConfig = baseTrans.TLSClientConfig
	}
	cli.Transport = newTransport
	return &cli, nil
}

func proxyTransport(proxyURL string) (*http.Transport, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		u = &url.URL{Host: proxyURL, Scheme: "http"}
	}
	return &http.Transport{
		Proxy: http.ProxyURL(u),
	}, nil
}

func WithProxyFromConfig(cli http.Client, dstHostOrURL string) (*http.Client, error) {
	proxyURL, ok := proxyFromConfig(dstHostOrURL)
	if !ok {
		return &cli, nil
	}
	return WithProxy(cli, proxyURL)
}

func proxyFromConfig(dstHostOrURL string) (string, bool) {
	host := URLToHost(dstHostOrURL)
	proxyURL, err := config.GetString(fmt.Sprintf("proxy:%s", host))
	if err != nil || proxyURL == "" {
		return "", false
	}
	return proxyURL, true
}
