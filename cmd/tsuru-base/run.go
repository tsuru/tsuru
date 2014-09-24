// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tsuru/tsuru/cmd"
	tsuruIo "github.com/tsuru/tsuru/io"
	"launchpad.net/gnuflag"
)

type AppRun struct {
	GuessingCommand
	once bool
}

type runFormatter struct{}

func (runFormatter) Format(out io.Writer, data []byte) error {
	var msg runMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return tsuruIo.ErrInvalidStreamChunk
	}
	if msg.Error != "" {
		return errors.New(msg.Error)
	}
	out.Write([]byte(msg.Message))
	return nil
}

type runMessage struct {
	Message string
	Error   string
}

func (c *AppRun) Info() *cmd.Info {
	desc := `run a command in all instances of the app, and prints the output.

If you use the '--once' flag tsuru will run the command only in one unit.

If you don't provide the app name, tsuru will try to guess it.
`
	return &cmd.Info{
		Name:    "run",
		Usage:   `run <command> [commandarg1] [commandarg2] ... [commandargn] [--app appname] [--once]`,
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *AppRun) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/run?once=%t", appName, c.once))
	if err != nil {
		return err
	}
	b := strings.NewReader(strings.Join(context.Args, " "))
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	w := tsuruIo.NewStreamWriter(context.Stdout, runFormatter{})
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(w, r.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := w.Remaining()
	if len(unparsed) > 0 {
		return fmt.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
}

func (c *AppRun) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = c.GuessingCommand.Flags()
		c.fs.BoolVar(&c.once, "once", false, "Running only one unit")
		c.fs.BoolVar(&c.once, "o", false, "Running only one unit")
	}
	return c.fs
}
