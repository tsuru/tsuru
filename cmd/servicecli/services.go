package main

import (
	"bytes"
	"fmt"
	"github.com/timeredbull/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
)

type ServiceCreate struct{}

func (c *ServiceCreate) Info() *cmd.Info {
	desc := "Creates a service based on a passed manifest. The manifest format should be a yaml and follow the standard described in the documentation (should link to it here)"
	return &cmd.Info{
		Name:    "create",
		Usage:   "create path/to/manifesto",
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *ServiceCreate) Run(context *cmd.Context, client cmd.Doer) error {
	manifest := context.Args[0]
	url := cmd.GetUrl("/services")
	b, err := ioutil.ReadFile(manifest)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	b, err = ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, string(b)+"\n")
	return nil
}

type ServiceRemove struct{}

func (c *ServiceRemove) Run(context *cmd.Context, client cmd.Doer) error {
	serviceName := context.Args[0]
	url := cmd.GetUrl("/services/" + serviceName)
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Service successfully removed.\n")
	return nil
}

func (c *ServiceRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Usage:   "remove <servicename>",
		Desc:    "removes a service from catalog",
		MinArgs: 1,
	}
}

type ServiceList struct{}

func (c *ServiceList) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "list",
		Usage: "list",
		Desc:  "list services that belongs to user's team and it's service instances.",
	}
}

func (c *ServiceList) Run(ctx *cmd.Context, client cmd.Doer) error {
	url := cmd.GetUrl("/services")
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	rslt, err := cmd.ShowServicesInstancesList(b)
	if err != nil {
		return err
	}
	ctx.Stdout.Write(rslt)
	return nil
}

type ServiceUpdate struct{}

func (c *ServiceUpdate) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update",
		Usage:   "service update <path/to/manifesto>",
		Desc:    "Update service data, extracting it from the given manifesto file.",
		MinArgs: 1,
	}
}

func (c *ServiceUpdate) Run(ctx *cmd.Context, client cmd.Doer) error {
	manifest := ctx.Args[0]
	b, err := ioutil.ReadFile(manifest)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", cmd.GetUrl("/services"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		io.WriteString(ctx.Stdout, "Service successfully updated.\n")
	}
	return nil
}

type ServiceAddDoc struct{}

func (c *ServiceAddDoc) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Usage:   "service doc add <service> <path/to/docfile>",
		Desc:    "Update service documentation, extracting it from the given file.",
		MinArgs: 2,
	}
}

func (c *ServiceAddDoc) Run(ctx *cmd.Context, client cmd.Doer) error {
	serviceName := ctx.Args[0]
	docPath := ctx.Args[1]
	b, err := ioutil.ReadFile(docPath)
	request, err := http.NewRequest("PUT", cmd.GetUrl("/services/"+serviceName+"/doc"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(ctx.Stdout, fmt.Sprintf("Documentation for '%s' successfully updated.\n", serviceName))
	return nil
}

type ServiceGetDoc struct{}

func (c *ServiceGetDoc) Run(ctx *cmd.Context, client cmd.Doer) error {
	serviceName := ctx.Args[0]
	request, err := http.NewRequest("GET", cmd.GetUrl("/services/"+serviceName+"/doc"), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	io.WriteString(ctx.Stdout, string(b)+"\n")
	return nil
}

func (c *ServiceGetDoc) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get",
		Usage:   "service doc get <service>",
		Desc:    "Shows service documentation.",
		MinArgs: 1,
	}
}
