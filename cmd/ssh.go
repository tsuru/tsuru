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

type sshToContainerCmd struct {
	GuessingCommand
}

func (c *sshToContainerCmd) Info() *Info {
	return &Info{
		Name:    "ssh",
		Usage:   "ssh [container-id] -a/--app <appname>",
		Desc:    "Open an SSH shell to the given container, or to one of the containers of the given app.",
		MinArgs: 0,
	}
}

func (c *sshToContainerCmd) Run(context *Context, client *Client) error {
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
	serverURL, err := GetURL(fmt.Sprintf("/apps/%s/ssh?%s", appName, queryString.Encode()))
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
	conn, err := net.Dial("tcp", parsedURL.Host)
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
