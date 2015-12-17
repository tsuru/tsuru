// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tsuru/gnuflag"
	"github.com/tsuru/tsuru/cmd"
)

type ListHealingHistoryCmd struct {
	fs            *gnuflag.FlagSet
	nodeOnly      bool
	containerOnly bool
}

func (c *ListHealingHistoryCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "docker-healing-list",
		Usage: "docker-healing-list [--node] [--container]",
		Desc:  "List healing history for nodes or containers.",
	}
}

func renderHistoryTable(history []HealingEvent, filter string, ctx *cmd.Context) {
	fmt.Fprintln(ctx.Stdout, strings.ToUpper(filter[:1])+filter[1:]+":")
	headers := cmd.Row([]string{"Start", "Finish", "Success", "Failing", "Created", "Error"})
	t := cmd.Table{Headers: headers}
	for i := len(history) - 1; i >= 0; i-- {
		event := history[i]
		if event.Action != filter+"-healing" {
			continue
		}
		data := make([]string, 2)
		if filter == "node" {
			data[0] = event.FailingNode.Address
			data[1] = event.CreatedNode.Address
		} else {
			data[0] = event.FailingContainer.ID
			data[1] = event.CreatedContainer.ID
			if len(data[0]) > 10 {
				data[0] = data[0][:10]
			}
			if len(data[1]) > 10 {
				data[1] = data[1][:10]
			}
		}
		t.AddRow(cmd.Row([]string{
			event.StartTime.Local().Format(time.Stamp),
			event.EndTime.Local().Format(time.Stamp),
			fmt.Sprintf("%t", event.Successful),
			data[0],
			data[1],
			event.Error,
		}))
	}
	t.LineSeparator = true
	t.Reverse()
	ctx.Stdout.Write(t.Bytes())
}

func (c *ListHealingHistoryCmd) Run(ctx *cmd.Context, client *cmd.Client) error {
	var filter string
	if c.nodeOnly && !c.containerOnly {
		filter = "node"
	}
	if c.containerOnly && !c.nodeOnly {
		filter = "container"
	}
	url, err := cmd.GetURL(fmt.Sprintf("/docker/healing?filter=%s", filter))
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var history []HealingEvent
	err = json.NewDecoder(resp.Body).Decode(&history)
	if err != nil {
		return err
	}
	if filter != "" {
		renderHistoryTable(history, filter, ctx)
	} else {
		renderHistoryTable(history, "node", ctx)
		renderHistoryTable(history, "container", ctx)
	}
	return nil
}

func (c *ListHealingHistoryCmd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("with-flags", gnuflag.ContinueOnError)
		c.fs.BoolVar(&c.nodeOnly, "node", false, "List only healing process started for nodes")
		c.fs.BoolVar(&c.containerOnly, "container", false, "List only healing process started for containers")
	}
	return c.fs
}
