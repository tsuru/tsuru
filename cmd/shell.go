// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

var httpHeaderRegexp = regexp.MustCompile(`HTTP/.*? (\d+)`)

type ShellToContainerCmd struct {
	GuessingCommand
}

func (c *ShellToContainerCmd) Info() *Info {
	return &Info{
		Name:  "app-shell",
		Usage: "app-shell [container-id] -a/--app <appname>",
		Desc: `Opens a remote shell inside container, using the API server as a proxy. You
can access an app container just giving app name.

Also, you can access a specific container from this app. In this case, you
have to specify part of the container's ID. You can list current container's
IDs using [[tsuru app-info]].


Open a remote shell to the given container, or to one of the containers of the given app.`,
		MinArgs: 0,
	}
}

func (c *ShellToContainerCmd) Run(context *Context, client *Client) error {
	var width, height int
	if stdin, ok := context.Stdin.(*os.File); ok {
		fd := int(stdin.Fd())
		if terminal.IsTerminal(fd) {
			width, height, _ = terminal.GetSize(fd)
			oldState, err := terminal.MakeRaw(fd)
			if err != nil {
				return err
			}
			defer terminal.Restore(fd, oldState)
			sigChan := make(chan os.Signal, 2)
			go func(c <-chan os.Signal) {
				if _, ok := <-c; ok {
					terminal.Restore(fd, oldState)
					os.Exit(1)
				}
			}(sigChan)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)
		}
	}
	queryString := make(url.Values)
	queryString.Set("width", strconv.Itoa(width))
	queryString.Set("height", strconv.Itoa(height))
	if len(context.Args) > 0 {
		queryString.Set("container", context.Args[0])
	}
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	serverURL, err := GetURL(fmt.Sprintf("/apps/%s/shell?%s", appName, queryString.Encode()))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", serverURL, nil)
	if err != nil {
		return err
	}
	request.Close = true
	token, err := ReadToken()
	if err == nil {
		request.Header.Set("Authorization", "bearer "+token)
	}
	parsedURL, _ := url.Parse(serverURL)
	host := parsedURL.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		port := "80"
		if parsedURL.Scheme == "https" {
			port = "443"
		}
		host += ":" + port
	}
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return err
	}
	defer conn.Close()
	request.Write(conn)
	bytesLimit := 12
	var readStr string
	byteBuffer := make([]byte, 1)
	for i := 0; i < bytesLimit && byteBuffer[0] != '\n'; i++ {
		_, err := conn.Read(byteBuffer)
		if err != nil {
			break
		}
		readStr += string(byteBuffer)
	}
	matches := httpHeaderRegexp.FindAllStringSubmatch(readStr, -1)
	if len(matches) > 0 && len(matches[0]) > 1 {
		return errors.New(strings.TrimSpace(readStr))
	} else {
		context.Stdout.Write([]byte(readStr))
	}
	errs := make(chan error, 2)
	quit := make(chan bool)
	go io.Copy(conn, context.Stdin)
	go func() {
		defer close(quit)
		_, err := io.Copy(context.Stdout, conn)
		if err != nil && err != io.EOF {
			errs <- err
		}
	}()
	<-quit
	close(errs)
	return <-errs
}
