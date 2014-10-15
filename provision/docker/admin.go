// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
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

	"code.google.com/p/go.crypto/ssh/terminal"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/errors"
	tsuruIo "github.com/tsuru/tsuru/io"
	"launchpad.net/gnuflag"
)

var httpHeaderRegexp = regexp.MustCompile(`HTTP/.*? (\d+)`)

type moveContainersCmd struct{}

type progressFormatter struct{}

func (progressFormatter) Format(out io.Writer, data []byte) error {
	var logEntry progressLog
	err := json.Unmarshal(data, &logEntry)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", logEntry.Message)
	return nil
}

func (c *moveContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "containers-move",
		Usage:   "containers-move <from host> <to host>",
		Desc:    "Move all containers from one host to another.\nThis command is especially useful for host maintenance.",
		MinArgs: 2,
	}
}

func (c *moveContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/containers/move")
	if err != nil {
		return err
	}
	params := map[string]string{
		"from": context.Args[0],
		"to":   context.Args[1],
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
}

type fixContainersCmd struct{}

func (fixContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "fix-containers",
		Usage: "fix-containers",
		Desc:  "Fix containers that are broken in the cluster.",
	}
}

func (fixContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/docker/fix-containers")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	return err
}

type moveContainerCmd struct{}

func (c *moveContainerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "container-move",
		Usage:   "container-move <container id> <to host>",
		Desc:    "Move specified container to another host.",
		MinArgs: 2,
	}
}

func (c *moveContainerCmd) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL(fmt.Sprintf("/docker/container/%s/move", context.Args[0]))
	if err != nil {
		return err
	}
	params := map[string]string{
		"to": context.Args[1],
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
}

type rebalanceContainersCmd struct {
	cmd.ConfirmationCommand
	fs  *gnuflag.FlagSet
	dry bool
}

func (c *rebalanceContainersCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "containers-rebalance",
		Usage:   "containers-rebalance [--dry] [-y/--assume-yes]",
		Desc:    "Move containers creating a more even distribution between docker nodes.",
		MinArgs: 0,
	}
}

func (c *rebalanceContainersCmd) Run(context *cmd.Context, client *cmd.Client) error {
	if !c.dry && !c.Confirm(context, "Are you sure you want to rebalance containers?") {
		return nil
	}
	url, err := cmd.GetURL("/docker/containers/rebalance")
	if err != nil {
		return err
	}
	params := map[string]string{
		"dry": fmt.Sprintf("%t", c.dry),
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	buffer := bytes.NewBuffer(b)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, progressFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, response.Body) {
	}
	return nil
}

func (c *rebalanceContainersCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.ConfirmationCommand.Flags()
		c.fs.BoolVar(&c.dry, "dry", false, "Dry run, only shows what would be done")
	}
	return c.fs
}

type sshToContainerCmd struct{}

func (sshToContainerCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ssh",
		Usage:   "ssh <container-id>",
		Desc:    "Open a SSH shell to the given container.",
		MinArgs: 1,
	}
}

func (sshToContainerCmd) Run(context *cmd.Context, _ *cmd.Client) error {
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
	serverURL, err := cmd.GetURL("/docker/ssh/" + context.Args[0] + "?" + queryString.Encode())
	if err != nil {
		return err
	}
	request, err := http.NewRequest("GET", serverURL, nil)
	if err != nil {
		return err
	}
	request.Close = true
	token, err := cmd.ReadToken()
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
	bytesLimit := 50
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
		code, _ := strconv.Atoi(matches[0][1])
		return &errors.HTTP{
			Code:    code,
			Message: strings.TrimSpace(readStr),
		}
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
