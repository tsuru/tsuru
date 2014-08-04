// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
	"sort"
	"strings"
)

type ServiceList struct{}

func (s ServiceList) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "service-list",
		Usage: "service-list",
		Desc:  "Get all available services, and user's instances for this services",
	}
}

func (s ServiceList) Run(ctx *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/services/instances")
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

type ServiceAdd struct {
	fs        *gnuflag.FlagSet
	teamOwner string
}

func (c *ServiceAdd) Info() *cmd.Info {
	usage := `service-add <servicename> <serviceinstancename> [plan] [-t/--owner-team <team>]
e.g.:

    $ tsuru service-add mongodb tsuru_mongodb small -t myteam

Will add a new instance of the "mongodb" service, named "tsuru_mongodb" with the plan "small".`
	return &cmd.Info{
		Name:    "service-add",
		Usage:   usage,
		Desc:    "Create a service instance to one or more apps make use of.",
		MinArgs: 2,
		MaxArgs: 3,
	}
}

func (c *ServiceAdd) Run(ctx *cmd.Context, client *cmd.Client) error {
	serviceName, instanceName := ctx.Args[0], ctx.Args[1]
	var plan string
	if len(ctx.Args) > 2 {
		plan = ctx.Args[2]
	}
	var b bytes.Buffer
	params := map[string]string{
		"name":         instanceName,
		"service_name": serviceName,
		"plan":         plan,
		"owner":        c.teamOwner,
	}
	err := json.NewEncoder(&b).Encode(params)
	if err != nil {
		return err
	}
	url, err := cmd.GetURL("/services/instances")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprint(ctx.Stdout, "Service successfully added.\n")
	return nil
}

func (c *ServiceAdd) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		flagDesc := "the team that owns te service (mandatory if the user is member of more than one team)"
		c.fs = gnuflag.NewFlagSet("service-add", gnuflag.ExitOnError)
		c.fs.StringVar(&c.teamOwner, "team-owner", "", flagDesc)
		c.fs.StringVar(&c.teamOwner, "t", "", flagDesc)
	}
	return c.fs
}

type ServiceBind struct {
	GuessingCommand
}

func (sb *ServiceBind) Run(ctx *cmd.Context, client *cmd.Client) error {
	appName, err := sb.Guess()
	if err != nil {
		return err
	}
	instanceName := ctx.Args[0]
	url, err := cmd.GetURL("/services/instances/" + instanceName + "/" + appName)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var variables []string
	dec := json.NewDecoder(resp.Body)
	msg := fmt.Sprintf("Instance %q is now bound to the app %q.\n", instanceName, appName)
	if err = dec.Decode(&variables); err == nil && len(variables) > 0 {
		msg += fmt.Sprintf(`
The following environment variables are now available for use in your app:

- %s

For more details, please check the documentation for the service, using service-doc command.
`, strings.Join(variables, "\n- "))
	}
	n, err := fmt.Fprint(ctx.Stdout, msg)
	if err != nil {
		return err
	}
	if n != len(msg) {
		return io.ErrShortWrite
	}
	return nil
}

func (sb *ServiceBind) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "bind",
		Usage: "bind <instancename> [--app appname]",
		Desc: `bind a service instance to an app

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

type ServiceUnbind struct {
	GuessingCommand
}

func (su *ServiceUnbind) Run(ctx *cmd.Context, client *cmd.Client) error {
	appName, err := su.Guess()
	if err != nil {
		return err
	}
	instanceName := ctx.Args[0]
	url, err := cmd.GetURL("/services/instances/" + instanceName + "/" + appName)
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
	msg := fmt.Sprintf("Instance %q is not bound to the app %q anymore.\n", instanceName, appName)
	n, err := fmt.Fprint(ctx.Stdout, msg)
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
		Name:  "unbind",
		Usage: "unbind <instancename> [--app appname]",
		Desc: `unbind a service instance from an app

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

type ServiceInstanceStatus struct{}

func (c ServiceInstanceStatus) Info() *cmd.Info {
	usg := `service-status <serviceinstancename>
e.g.:

    $ tsuru service-status my_mongodb
`
	return &cmd.Info{
		Name:    "service-status",
		Usage:   usg,
		Desc:    "Check status of a given service instance.",
		MinArgs: 1,
	}
}

func (c ServiceInstanceStatus) Run(ctx *cmd.Context, client *cmd.Client) error {
	instName := ctx.Args[0]
	url, err := cmd.GetURL("/services/instances/" + instName + "/status")
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
	defer resp.Body.Close()
	bMsg, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	msg := string(bMsg) + "\n"
	n, err := fmt.Fprint(ctx.Stdout, msg)
	if err != nil {
		return err
	}
	if n != len(msg) {
		return errors.New("Failed to write to standard output.\n")
	}
	return nil
}

type ServiceInfo struct{}

func (c ServiceInfo) Info() *cmd.Info {
	usg := `service-info <service>
e.g.:

    $ tsuru service-info mongodb
`
	return &cmd.Info{
		Name:    "service-info",
		Usage:   usg,
		Desc:    "List all instances of a service",
		MinArgs: 1,
	}
}

type ServiceInstanceModel struct {
	Name string
	Apps []string
	Info map[string]string
}

// in returns true if the list contains the value
func in(value string, list []string) bool {
	for _, item := range list {
		if value == item {
			return true
		}
	}
	return false
}

func (ServiceInfo) ExtraHeaders(instances []ServiceInstanceModel) []string {
	var headers []string
	for _, instance := range instances {
		for key := range instance.Info {
			if !in(key, headers) {
				headers = append(headers, key)
			}
		}
	}
	sort.Sort(sort.StringSlice(headers))
	return headers
}

func (c ServiceInfo) BuildInstancesTable(serviceName string, ctx *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL("/services/" + serviceName)
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
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var instances []ServiceInstanceModel
	err = json.Unmarshal(result, &instances)
	if err != nil {
		return err
	}
	ctx.Stdout.Write([]byte(fmt.Sprintf("Info for \"%s\"\n\n", serviceName)))
	if len(instances) > 0 {
		ctx.Stdout.Write([]byte("Instances\n"))
		table := cmd.NewTable()
		extraHeaders := c.ExtraHeaders(instances)
		for _, instance := range instances {
			apps := strings.Join(instance.Apps, ", ")
			data := []string{instance.Name, apps}
			for _, h := range extraHeaders {
				data = append(data, instance.Info[h])
			}
			table.AddRow(cmd.Row(data))
		}
		headers := []string{"Instances", "Apps"}
		headers = append(headers, extraHeaders...)
		table.Headers = cmd.Row(headers)
		ctx.Stdout.Write(table.Bytes())
	}
	return nil
}

func (c ServiceInfo) BuildPlansTable(serviceName string, ctx *cmd.Context, client *cmd.Client) error {
	ctx.Stdout.Write([]byte("\nPlans\n"))
	url, err := cmd.GetURL(fmt.Sprintf("/services/%s/plans", serviceName))
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
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var plans []map[string]string
	err = json.Unmarshal(result, &plans)
	if err != nil {
		return err
	}
	if len(plans) > 0 {
		table := cmd.NewTable()
		for _, plan := range plans {
			data := []string{plan["Name"], plan["Description"]}
			table.AddRow(cmd.Row(data))
		}
		table.Headers = cmd.Row([]string{"Name", "Description"})
		ctx.Stdout.Write(table.Bytes())
	}
	return nil
}

func (c ServiceInfo) Run(ctx *cmd.Context, client *cmd.Client) error {
	serviceName := ctx.Args[0]
	err := c.BuildInstancesTable(serviceName, ctx, client)
	if err != nil {
		return err
	}
	return c.BuildPlansTable(serviceName, ctx, client)
}

type ServiceDoc struct{}

func (ServiceDoc) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "service-doc",
		Usage:   "service-doc <servicename>",
		Desc:    "Show documentation of a service",
		MinArgs: 1,
	}
}

func (ServiceDoc) Run(ctx *cmd.Context, client *cmd.Client) error {
	sName := ctx.Args[0]
	url := fmt.Sprintf("/services/%s/doc", sName)
	url, err := cmd.GetURL(url)
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
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	ctx.Stdout.Write(result)
	return nil
}

type ServiceRemove struct {
	yes bool
	fs  *gnuflag.FlagSet
}

func (c *ServiceRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "service-remove",
		Usage:   "service-remove <serviceinstancename> [--assume-yes]",
		Desc:    "Removes a service instance",
		MinArgs: 1,
	}
}

func (c *ServiceRemove) Run(ctx *cmd.Context, client *cmd.Client) error {
	name := ctx.Args[0]
	var answer string
	if !c.yes {
		fmt.Fprintf(ctx.Stdout, `Are you sure you want to remove service "%s"? (y/n) `, name)
		fmt.Fscanf(ctx.Stdin, "%s", &answer)
		if answer != "y" {
			fmt.Fprintln(ctx.Stdout, "Abort.")
			return nil
		}
	}
	url := fmt.Sprintf("/services/instances/%s", name)
	url, err := cmd.GetURL(url)
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
	fmt.Fprintf(ctx.Stdout, `Service "%s" successfully removed!`+"\n", name)
	return nil
}

func (c *ServiceRemove) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("service-remove", gnuflag.ExitOnError)
		c.fs.BoolVar(&c.yes, "assume-yes", false, "Don't ask for confirmation, just remove the service.")
		c.fs.BoolVar(&c.yes, "y", false, "Don't ask for confirmation, just remove the service.")
	}
	return c.fs
}
