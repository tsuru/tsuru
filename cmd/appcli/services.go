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
		Usage:   "service (add|list|bind|unbind|instance|doc)",
		Desc:    "manage your services",
		MinArgs: 1,
	}
}

func (s *Service) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"list":     &ServiceList{},
		"add":      &ServiceAdd{},
		"bind":     &ServiceBind{},
		"info":     &ServiceInfo{},
		"unbind":   &ServiceUnbind{},
		"instance": &ServiceInstance{},
		"doc":      &ServiceDoc{},
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

type ServiceInstance struct{}

func (c *ServiceInstance) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "instance",
		Usage:   "service instance (status)",
		Desc:    "Retrieve information about services instances",
		MinArgs: 1,
	}
}

func (s *ServiceInstance) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"status": &ServiceInstanceStatus{},
	}
}

type ServiceInstanceStatus struct{}

func (c *ServiceInstanceStatus) Info() *cmd.Info {
	usg := `service instance status <serviceinstancename>
e.g.:

    $ service instance status my_mongodb
`
	return &cmd.Info{
		Name:    "status",
		Usage:   usg,
		Desc:    "Check status of a given service instance.",
		MinArgs: 1,
	}
}

func (c *ServiceInstanceStatus) Run(ctx *cmd.Context, client cmd.Doer) error {
	instName := ctx.Args[0]
	url := cmd.GetUrl("/services/instances/" + instName + "/status")
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
	n, err := io.WriteString(ctx.Stdout, msg)
	if err != nil {
		return err
	}
	if n != len(msg) {
		return errors.New("Failed to write to standard output.\n")
	}
	return nil
}

type ServiceInfo struct{}

func (c *ServiceInfo) Info() *cmd.Info {
	usg := `service info <service>
e.g.:

    $ service info mongodb
`
	return &cmd.Info{
		Name:    "info",
		Usage:   usg,
		Desc:    "List all instances of a service",
		MinArgs: 1,
	}
}

type ServiceInstanceModel struct {
	Name string
	Apps []string
}

func (c *ServiceInfo) Run(ctx *cmd.Context, client cmd.Doer) error {
	serviceName := ctx.Args[0]
	url := cmd.GetUrl("/services/" + serviceName)
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
	ctx.Stdout.Write([]byte(fmt.Sprintf("Info for \"%s\"\n", serviceName)))
	if len(instances) > 0 {
		table := cmd.NewTable()
		table.Headers = cmd.Row([]string{"Instances", "Apps"})
		for _, instance := range instances {
			apps := strings.Join(instance.Apps, ", ")
			table.AddRow(cmd.Row([]string{instance.Name, apps}))
		}
		ctx.Stdout.Write(table.Bytes())
	}
	return nil
}

type ServiceDoc struct{}

func (c *ServiceDoc) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "doc",
		Usage:   "service doc <servicename>",
		Desc:    "Show documentation of a service",
		MinArgs: 1,
	}
}

func (c *ServiceDoc) Run(ctx *cmd.Context, client cmd.Doer) error {
	sName := ctx.Args[0]
	url := fmt.Sprintf("/services/c/%s/doc", sName)
	url = cmd.GetUrl(url)
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
	result = append(result, []byte("\n")...)
	ctx.Stdout.Write(result)
	return nil
}
