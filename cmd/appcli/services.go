package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
)

type Service struct{}

func (s *Service) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "service",
		Usage:   "service (add|list|bind|unbind)",
		Desc:    "manage your services",
		MinArgs: 1,
	}
}

func (s *Service) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"list":   &ServiceList{},
		"add":    &ServiceAdd{},
		"bind":   &ServiceBind{},
		"unbind": &ServiceUnbind{},
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
	req, err := http.NewRequest("GET", cmd.GetUrl("/services/instances"), nil)
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
	rslt, err := cmd.ShowServicesInstancesList(b)
	if err != nil {
		return err
	}
	n, err := ctx.Stdout.Write(rslt)
	if n != len(rslt) {
		return errors.New("Failed to write the output of the command")
	}
	return nil
}

type ServiceAdd struct{}

func (sa *ServiceAdd) Info() *cmd.Info {
	usage := `service add <servicename> <serviceinstancename>
e.g.:

    $ service add mongodb tsuru_mongodb

Will add a new instance of the "mongodb" service, named "tsuru_mongodb".`
	return &cmd.Info{
		Name:    "add",
		Usage:   usage,
		Desc:    "Create a service instance to one or more apps make use of.",
		MinArgs: 2,
	}
}

func (sa *ServiceAdd) Run(ctx *cmd.Context, client cmd.Doer) error {
	srvName, instName := ctx.Args[0], ctx.Args[1]
	fmtBody := fmt.Sprintf(`{"name": "%s", "service_name": "%s"}`, instName, srvName)
	b := bytes.NewBufferString(fmtBody)
	url := cmd.GetUrl("/services/instances")
	request, err := http.NewRequest("POST", url, b)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(ctx.Stdout, "service successfully added.\n")
	return nil
}

type ServiceBind struct{}

func (sb *ServiceBind) Run(ctx *cmd.Context, client cmd.Doer) error {
	instanceName, appName := ctx.Args[0], ctx.Args[1]
	url := cmd.GetUrl("/services/instances/" + instanceName + "/" + appName)
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("Instance %s successfully binded to the app %s.\n", instanceName, appName)
	n, err := io.WriteString(ctx.Stdout, msg)
	if err != nil {
		return err
	}
	if n != len(msg) {
		return errors.New("Failed to write to standard output.\n")
	}
	return nil
}

func (sb *ServiceBind) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bind",
		Usage:   "service bind <instancename> <appname>",
		Desc:    "bind a service instance to an app",
		MinArgs: 2,
	}
}

type ServiceUnbind struct{}

func (su *ServiceUnbind) Run(ctx *cmd.Context, client cmd.Doer) error {
	instanceName, appName := ctx.Args[0], ctx.Args[1]
	url := cmd.GetUrl("/services/instances/" + instanceName + "/" + appName)
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("Instance %s successfully unbinded from the app %s.\n", instanceName, appName)
	n, err := io.WriteString(ctx.Stdout, msg)
	if err != nil {
		return err
	}
	if n != len(msg) {
		return errors.New("Failed to write to standard output.\n")
	}
	return nil
}

func (su *ServiceUnbind) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unbind",
		Usage:   "service unbind <instancename> <appname>",
		Desc:    "unbind a service instance from an app",
		MinArgs: 2,
	}
}
