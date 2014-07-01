// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io/ioutil"
	"net/http"
	"os"
)

type ServiceCreate struct{}

func (c *ServiceCreate) Info() *cmd.Info {
	desc := "Creates a service based on a passed manifest. The manifest format should be a yaml and follow the standard described in the documentation (should link to it here)"
	return &cmd.Info{
		Name:    "create",
		Usage:   "create path/to/manifest [- for stdin]",
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *ServiceCreate) Run(context *cmd.Context, client *cmd.Client) error {
	manifest := context.Args[0]
	url, err := cmd.GetURL("/services")
	if err != nil {
		return err
	}
	var data []byte
	if manifest == "-" {
		data, err = ioutil.ReadAll(os.Stdin)
	} else {
		data, err = ioutil.ReadFile(manifest)
	}
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "%s", b)
	return nil
}

type ServiceRemove struct{}

func (c *ServiceRemove) Run(context *cmd.Context, client *cmd.Client) error {
	serviceName := context.Args[0]
	url, err := cmd.GetURL("/services/" + serviceName)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Service successfully removed.")
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

func (c *ServiceList) Run(ctx *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/services")
	if err != nil {
		return err
	}
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
		Usage:   "update <path/to/manifest>",
		Desc:    "Update service data, extracting it from the given manifest file.",
		MinArgs: 1,
	}
}

func (c *ServiceUpdate) Run(ctx *cmd.Context, client *cmd.Client) error {
	manifest := ctx.Args[0]
	b, err := ioutil.ReadFile(manifest)
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/services")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Fprintln(ctx.Stdout, "Service successfully updated.")
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

func (c *ServiceDocAdd) Run(ctx *cmd.Context, client *cmd.Client) error {
	serviceName := ctx.Args[0]
	url, err := cmd.GetURL("/services/" + serviceName + "/doc")
	if err != nil {
		return err
	}
	docPath := ctx.Args[1]
	b, err := ioutil.ReadFile(docPath)
	request, err := http.NewRequest("PUT", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "Documentation for '%s' successfully updated.\n", serviceName)
	return nil
}

type ServiceDocGet struct{}

func (c *ServiceDocGet) Run(ctx *cmd.Context, client *cmd.Client) error {
	serviceName := ctx.Args[0]
	url, err := cmd.GetURL("/services/" + serviceName + "/doc")
	if err != nil {
		return err
	}
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
	ctx.Stdout.Write(b)
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

const passwordSize = 12

func generatePassword() (string, error) {
	b := make([]byte, passwordSize)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (c *ServiceTemplate) Run(ctx *cmd.Context, client *cmd.Client) error {
	pass, err := generatePassword()
	if err != nil {
		return err
	}
	template := `id: servicename
password: %s
endpoint:
  production: production-endpoint.com
  test: test-endpoint.com:8080`
	template = fmt.Sprintf(template, pass)
	f, err := os.Create("manifest.yaml")
	defer f.Close()
	if err != nil {
		return errors.New("Error while creating manifest template.\nOriginal error message is: " + err.Error())
	}
	f.Write([]byte(template))
	fmt.Fprintln(ctx.Stdout, `Generated file "manifest.yaml" in current path`)
	return nil
}
