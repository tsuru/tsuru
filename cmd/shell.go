// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"github.com/tsuru/gnuflag"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/websocket"
)

var httpRegexp = regexp.MustCompile(`^http`)

type ShellToContainerCmd struct {
	AppNameMixIn
	isolated bool
	fs       *gnuflag.FlagSet
}

func (c *ShellToContainerCmd) Info() *Info {
	return &Info{
		Name:  "app-shell",
		Usage: "app-shell [unit-id] -a/--app <appname> [-i/--isolated]",
		Desc: `Opens a remote shell inside unit, using the API server as a proxy. You
can access an app unit just giving app name, or specifying the id of the unit.
You can get the ID of the unit using the app-info command.`,
		MinArgs: 0,
	}
}

func (c *ShellToContainerCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.AppNameMixIn.Flags()
		help := "Run shell in a new unit"
		c.fs.BoolVar(&c.isolated, "isolated", false, help)
		c.fs.BoolVar(&c.isolated, "i", false, help)
	}
	return c.fs
}

func (c *ShellToContainerCmd) Run(context *Context, client *Client) error {
	appName, err := c.AppName()
	if err != nil {
		return err
	}
	appInfoURL, err := GetURL(fmt.Sprintf("/apps/%s", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", appInfoURL, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	context.RawOutput()
	var width, height int
	if desc, ok := context.Stdin.(descriptable); ok {
		fd := int(desc.Fd())
		if terminal.IsTerminal(fd) {
			width, height, _ = terminal.GetSize(fd)
			oldState, terminalErr := terminal.MakeRaw(fd)
			if terminalErr != nil {
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
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		}
	}
	queryString := make(url.Values)
	queryString.Set("isolated", strconv.FormatBool(c.isolated))
	queryString.Set("width", strconv.Itoa(width))
	queryString.Set("height", strconv.Itoa(height))
	if len(context.Args) > 0 {
		queryString.Set("unit", context.Args[0])
		queryString.Set("container_id", context.Args[0])
	}
	if term := os.Getenv("TERM"); term != "" {
		queryString.Set("term", term)
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
	var token string
	if token, err = ReadToken(); err == nil {
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
