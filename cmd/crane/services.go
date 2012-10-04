package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
	"os"
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
		Usage:   "update <path/to/manifesto>",
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

type ServiceDocAdd struct{}

func (c *ServiceDocAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "doc-add",
		Usage:   "service doc-add <service> <path/to/docfile>",
		Desc:    "Update service documentation, extracting it from the given file.",
		MinArgs: 2,
	}
}

func (c *ServiceDocAdd) Run(ctx *cmd.Context, client cmd.Doer) error {
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

type ServiceDocGet struct{}

func (c *ServiceDocGet) Run(ctx *cmd.Context, client cmd.Doer) error {
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
	io.WriteString(ctx.Stdout, string(b))
	return nil
}

func (c *ServiceDocGet) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "doc-get",
		Usage:   "service doc-get <service>",
		Desc:    "Shows service documentation.",
		MinArgs: 1,
	}
}

type ServiceTemplate struct{}

func (c *ServiceTemplate) Info() *cmd.Info {
	usg := `template
e.g.: $ crane template`
	return &cmd.Info{
		Name:  "template",
		Usage: usg,
		Desc:  "Generates a manifest template file and places it in current path",
	}
}

func (c *ServiceTemplate) Run(ctx *cmd.Context, client cmd.Doer) error {
	template := `id: servicename
endpoint:
  production: production-endpoint.com
  test: test-endpoint.com:8080`
	f, err := os.Create("manifest.yaml")
	defer f.Close()
	if err != nil {
		return errors.New("Error while creating manifest template.\nOriginal error message is: " + err.Error())
	}
	f.Write([]byte(template))
	io.WriteString(ctx.Stdout, "Generated file \"manifest.yaml\" in current path\n")
	return nil
}
