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
		"get": &EnvGet{},
		"set": &EnvSet{},
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
	appName, vars := context.Args[0], context.Args[1:]
	varsStr := strings.Join(vars, " ")
	url := GetUrl(fmt.Sprintf("/apps/%s/env", appName))
	body := strings.NewReader(varsStr)
	request, err := http.NewRequest("GET", url, body)
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
	appName, vars := context.Args[0], context.Args[1:]
	varsStr := strings.Join(vars, " ")
	url := GetUrl(fmt.Sprintf("/apps/%s/env", appName))
	body := strings.NewReader(varsStr)
	request, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "variable(s) successfuly exported")
	return nil
}
