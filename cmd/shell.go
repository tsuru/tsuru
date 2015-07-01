// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/websocket"
)

var httpHeaderRegexp = regexp.MustCompile(`HTTP/.*? (\d+)`)

var httpRegexp = regexp.MustCompile(`^http`)

type ShellToContainerCmd struct {
	GuessingCommand
}

func (c *ShellToContainerCmd) Info() *Info {
	return &Info{
		Name:  "app-shell",
		Usage: "app-shell [unit-id] -a/--app <appname>",
		Desc: `Opens a remote shell inside unit, using the API server as a proxy. You
can access an app unit just giving app name, or specifying the id of the unit.
You can get the ID of the unit using the app-info command.`,
		MinArgs: 0,
	}
}

func (c *ShellToContainerCmd) Run(context *Context, client *Client) error {
	context.RawOutput()
	var width, height int
	if desc, ok := context.Stdin.(descriptable); ok {
		fd := int(desc.Fd())
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
		queryString.Set("unit", context.Args[0])
		queryString.Set("container_id", context.Args[0])
	}
	if term := os.Getenv("TERM"); term != "" {
		queryString.Set("term", term)
	}
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	serverURL, err := GetURL(fmt.Sprintf("/apps/%s/shell?%s", appName, queryString.Encode()))
	if err != nil {
		return err
	}
	serverURL = httpRegexp.ReplaceAllString(serverURL, "ws")
	config, err := websocket.NewConfig(serverURL, "ws://localhost")
	if err != nil {
		return err
	}
	if token, err := ReadToken(); err == nil {
		config.Header.Set("Authorization", "bearer "+token)
	}
	conn, err := websocket.DialConfig(config)
	if err != nil {
		return err
	}
	defer conn.Close()
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
