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

func open(url string) error {
	if runtime.GOOS == "linux" {
		return exec.Command("xdg-open", url).Start()
	}
	return exec.Command("open", url).Start()
}

func serve() (string, error) {
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Test")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		url := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=http://localhost:4242/callback&scope=user:email", clientID())
		http.Redirect(w, r, url, 302)
	})
	l, e := net.Listen("tcp", ":0")
	if e != nil {
		return "", e
	}
	server := &http.Server{}
	return l.Addr().String(), server.Serve(l)
}

func startServerAndOpenBrowser() {
	url, err := func() (string, error) {
		return serve()
	}()
	if err != nil {
		return
	}
	open(url)
}

func oauthLogin(context *Context, client *Client) error {
	startServerAndOpenBrowser()
	return nil
}
