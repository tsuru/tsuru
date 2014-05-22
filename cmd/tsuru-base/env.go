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
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const envSetValidationMessage = `You must specify environment variables in the form "NAME=value".

Example:

  tsuru env-set NAME=value OTHER_NAME=value with spaces ANOTHER_NAME="using quotes"`

type EnvGet struct {
	GuessingCommand
}

func (c *EnvGet) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-get",
		Usage: "env-get [--app appname] [ENVIRONMENT_VARIABLE1] [ENVIRONMENT_VARIABLE2] ...",
		Desc: `retrieve environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 0,
	}
}

func (c *EnvGet) Run(context *cmd.Context, client *cmd.Client) error {
	b, err := requestEnvURL("GET", c.GuessingCommand, context.Args, client)
	if err != nil {
		return err
	}
	var variables []map[string]interface{}
	err = json.Unmarshal(b, &variables)
	if err != nil {
		return err
	}
	formatted := make([]string, 0, len(variables))
	for _, v := range variables {
		value := "*** (private variable)"
		if v["public"].(bool) {
			value = v["value"].(string)
		}
		formatted = append(formatted, fmt.Sprintf("%s=%s", v["name"], value))
	}
	sort.Strings(formatted)
	fmt.Fprintln(context.Stdout, strings.Join(formatted, "\n"))
	return nil
}

type EnvSet struct {
	GuessingCommand
}

func (c *EnvSet) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-set",
		Usage: "env-set <NAME=value> [NAME=value] ... [--app appname]",
		Desc: `set environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *EnvSet) Run(context *cmd.Context, client *cmd.Client) error {
	appName, err := c.Guess()
	if err != nil {
		return err
	}
	raw := strings.Join(context.Args, " ")
	regex := regexp.MustCompile(`(\w+=[^\s]+[^=]+)(\s|$)`)
	decls := regex.FindAllStringSubmatch(raw, -1)
	if len(decls) < 1 {
		return errors.New(envSetValidationMessage)
	}
	variables := make(map[string]string, len(decls))
	for _, v := range decls {
		parts := strings.Split(v[1], "=")
		variables[parts[0]] = strings.Join(parts[1:], "=")
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(variables)
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/env", appName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully exported\n")
	return nil
}

type EnvUnset struct {
	GuessingCommand
}

func (c *EnvUnset) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "env-unset",
		Usage: "env-unset <ENVIRONMENT_VARIABLE1> [ENVIRONMENT_VARIABLE2] ... [ENVIRONMENT_VARIABLEN] [--app appname]",
		Desc: `unset environment variables for an app.

If you don't provide the app name, tsuru will try to guess it.`,
		MinArgs: 1,
	}
}

func (c *EnvUnset) Run(context *cmd.Context, client *cmd.Client) error {
	_, err := requestEnvURL("DELETE", c.GuessingCommand, context.Args, client)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "variable(s) successfully unset\n")
	return nil
}

func requestEnvURL(method string, g GuessingCommand, args []string, client *cmd.Client) ([]byte, error) {
	appName, err := g.Guess()
	if err != nil {
		return nil, err
	}
	url, err := cmd.GetURL(fmt.Sprintf("/apps/%s/env", appName))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(args)
	request, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	r, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}
