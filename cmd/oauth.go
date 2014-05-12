// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

func clientID() string {
	return os.Getenv("TSURU_AUTH_CLIENTID")
}

func port() string {
	p := os.Getenv("TSURU_AUTH_SERVER_PORT")
	if p == "" {
		return ":0"
	}
	return p
}

func open(url string) error {
	if runtime.GOOS == "linux" {
		return exec.Command("xdg-open", url).Start()
	}
	return exec.Command("open", url).Start()
}

func serve(url chan string, finish chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		fmt.Fprintf(w, "Test")
	})
	l, e := net.Listen("tcp", port())
	if e != nil {
		return
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	url <- fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=http://localhost:%s&scope=user:email", clientID(), port)
	server := &http.Server{}
	server.Serve(l)
}

func startServerAndOpenBrowser() {
	url := make(chan string)
	finish := make(chan bool)
	go func() {
		serve(url, finish)
	}()
	open(<-url)
	<-finish
}

func oauthLogin(context *Context, client *Client) error {
	startServerAndOpenBrowser()
	return nil
}
