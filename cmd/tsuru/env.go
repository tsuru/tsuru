package main

import (
	"fmt"
	"github.com/timeredbull/tsuru/cmd"
	"io/ioutil"
	"net/http"
	"strings"
)

type EnvGet struct{}

func (c *EnvGet) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "env-get",
		Usage:   "env-get <appname> [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...",
		Desc:    "retrieve environment variables for an app.",
		MinArgs: 1,
	}
}

func (c *EnvGet) Run(context *cmd.Context, client cmd.Doer) error {
	b, err := requestEnvUrl("GET", context.Args, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, string(b))
	return nil
}

type EnvSet struct{}

func (c *EnvSet) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "env-set",
		Usage:   "env-set <appname> <NAME=value> [NAME=value] ...",
		Desc:    "set environment variables for an app.",
		MinArgs: 2,
	}
}

func (c *EnvSet) Run(context *cmd.Context, client cmd.Doer) error {
	_, err := requestEnvUrl("POST", context.Args, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully exported\n")
	return nil
}

type EnvUnset struct{}

func (c *EnvUnset) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "env-unset",
		Usage:   "env-unset <appname> <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2]",
		Desc:    "unset environment variables for an app.",
		MinArgs: 2,
	}
}

func (c *EnvUnset) Run(context *cmd.Context, client cmd.Doer) error {
	_, err := requestEnvUrl("DELETE", context.Args, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully unset\n")
	return nil
}

func requestEnvUrl(method string, args []string, client cmd.Doer) (string, error) {
	appName, vars := args[0], args[1:]
	varsStr := strings.Join(vars, " ")
	url := cmd.GetUrl(fmt.Sprintf("/apps/%s/env", appName))
	body := strings.NewReader(varsStr)
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return "", err
	}
	r, err := client.Do(request)
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
