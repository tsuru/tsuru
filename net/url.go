// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"net"
	"net/url"
)

func URLToHost(urlStr string) string {
	var urlHost string
	url, _ := url.Parse(urlStr)
	if url == nil || url.Host == "" {
		urlHost = urlStr
	} else {
		urlHost = url.Host
	}
	host, _, _ := net.SplitHostPort(urlHost)
	if host == "" {
		return urlHost
	}
	return host
}
