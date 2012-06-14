package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type Env struct{}

func (c *Env) Info() *Info {
	return &Info{
		Name:  "env",
		Usage: "env (get|set|unset)",
		Desc:  "manage instance's environment variables.",
	}
}

func (c *Env) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"get":   &EnvGet{},
		"set":   &EnvSet{},
		"unset": &EnvUnset{},
	}
}

type EnvGet struct{}

func (c *EnvGet) Info() *Info {
	return &Info{
		Name:  "get",
		Usage: "env get appname envname",
		Desc:  "retrieve environment variables for an app.",
	}
}

func (c *EnvGet) Run(context *Context, client Doer) error {
	b, err := requestEnvUrl("GET", context.Args, client)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, string(b))
	return nil
}

type EnvSet struct{}

func (c *EnvSet) Info() *Info {
	return &Info{
		Name:  "set",
		Usage: "env set appname envname",
		Desc:  "set environment variables for an app.",
	}
}

func (c *EnvSet) Run(context *Context, client Doer) error {
	_, err := requestEnvUrl("POST", context.Args, client)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "variable(s) successfuly exported")
	return nil
}

type EnvUnset struct{}

func (c *EnvUnset) Info() *Info {
	return &Info{
		Name:  "unset",
		Usage: "env unset appname envname",
		Desc:  "unset environment variables for an app.",
	}
}

func (c *EnvUnset) Run(context *Context, client Doer) error {
	_, err := requestEnvUrl("DELETE", context.Args, client)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "variable(s) successfuly unset")
	return nil
}

func requestEnvUrl(method string, args []string, client Doer) (string, error) {
	appName, vars := args[0], args[1:]
	varsStr := strings.Join(vars, " ")
	url := GetUrl(fmt.Sprintf("/apps/%s/env", appName))
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
