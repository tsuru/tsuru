package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type Service struct{}

func (s *Service) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "service",
		Usage:   "service (list)",
		Desc:    "manage your services",
		MinArgs: 1,
	}
}

func (s *Service) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"list": &ServiceList{},
		"add":  &ServiceAdd{},
	}
}

type ServiceList struct{}

func (s *ServiceList) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "list",
		Usage: "service list",
		Desc:  "Get all available services, and user's instances for this services",
	}
}

func (s *ServiceList) Run(ctx *cmd.Context, client cmd.Doer) error {
	req, err := http.NewRequest("GET", cmd.GetUrl("/services"), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var body map[string][]string
	err = json.Unmarshal(b, &body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	table := cmd.NewTable()
	table.Headers = cmd.Row([]string{"Service", "Instances"})
	for s, i := range body {
		instances := strings.Join(i, ", ")
		table.AddRow(cmd.Row([]string{s, instances}))
	}
	content := table.Bytes()
	n, err := ctx.Stdout.Write(content)
	if n != len(content) {
		return errors.New("Failed to write the output of the command")
	}
	return err
}

type ServiceAdd struct{}

func (sa *ServiceAdd) Info() *cmd.Info {
	usage := `service add appname serviceinstancename servicename
    e.g.:
    $ service add tsuru tsuru_db mongodb`
	return &cmd.Info{
		Name:    "add",
		Usage:   usage,
		Desc:    "Create a service instance to one or more apps make use of.",
		MinArgs: 3,
	}
}

func (sa *ServiceAdd) Run(ctx *cmd.Context, client cmd.Doer) error {
	appName, instName, srvName := ctx.Args[0], ctx.Args[1], ctx.Args[2]
	fmtBody := fmt.Sprintf(`{"app": "%s", "name": "%s", "service_name": "%s"}`, appName, instName, srvName)
	b := bytes.NewBufferString(fmtBody)
	url := cmd.GetUrl("/services/instances")
	request, err := http.NewRequest("POST", url, b)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	io.WriteString(ctx.Stdout, string(result))
	return nil
}
