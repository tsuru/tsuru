// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/cmd/term"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"net/http"
	"os"
	"sort"
)

type userCreate struct{}

func (c *userCreate) Info() *Info {
	return &Info{
		Name:    "user-create",
		Usage:   "user-create <email>",
		Desc:    "creates a user.",
		MinArgs: 1,
	}
}

func (c *userCreate) Run(context *Context, client *Client) error {
	url, err := GetURL("/users")
	if err != nil {
		return err
	}
	email := context.Args[0]
	fmt.Fprint(context.Stdout, "Password: ")
	password, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "\nConfirm: ")
	confirm, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout)
	if password != confirm {
		return errors.New("Passwords didn't match.")
	}
	b := bytes.NewBufferString(`{"email":"` + email + `", "password":"` + password + `"}`)
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if resp != nil {
		if resp.StatusCode == http.StatusNotFound ||
			resp.StatusCode == http.StatusMethodNotAllowed {
			return errors.New("User creation is disabled.")
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `User "%s" successfully created!`+"\n", email)
	return nil
}

type userRemove struct{}

func (c *userRemove) Run(context *Context, client *Client) error {
	var answer string
	fmt.Fprint(context.Stdout, `Are you sure you want to remove your user from tsuru? (y/n) `)
	fmt.Fscanf(context.Stdin, "%s", &answer)
	if answer != "y" {
		fmt.Fprintln(context.Stdout, "Abort.")
		return nil
	}
	url, err := GetURL("/users")
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
	filesystem().Remove(JoinWithUserDir(".tsuru_token"))
	fmt.Fprint(context.Stdout, "User successfully removed.\n")
	return nil
}

func (c *userRemove) Info() *Info {
	return &Info{
		Name:    "user-remove",
		Usage:   "user-remove",
		Desc:    "removes your user from tsuru server.",
		MinArgs: 0,
	}
}

type login struct{}

func nativeLogin(context *Context, client *Client) error {
	email := context.Args[0]
	fmt.Fprint(context.Stdout, "Password: ")
	password, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout)
	url, err := GetURL("/users/" + email + "/tokens")
	if err != nil {
		return err
	}
	b := bytes.NewBufferString(`{"password":"` + password + `"}`)
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	out := make(map[string]interface{})
	err = json.Unmarshal(result, &out)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Successfully logged in!")
	return writeToken(out["token"].(string))
}

func (c *login) Run(context *Context, client *Client) error {
	if scheme() == "oauth" {
		return oauthLogin(context, client)
	}
	return nativeLogin(context, client)
}

func (c *login) Info() *Info {
	args := 1
	usage := "login <email>"
	if scheme() == "oauth" {
		usage = "login"
		args = 0
	}
	return &Info{
		Name:    "login",
		Usage:   usage,
		Desc:    "log in with your credentials.",
		MinArgs: args,
	}
}

type logout struct{}

func (c *logout) Info() *Info {
	return &Info{
		Name:  "logout",
		Usage: "logout",
		Desc:  "clear local authentication credentials.",
	}
}

func (c *logout) Run(context *Context, client *Client) error {
	if url, err := GetURL("/users/tokens"); err == nil {
		request, _ := http.NewRequest("DELETE", url, nil)
		client.Do(request)
	}
	err := filesystem().Remove(JoinWithUserDir(".tsuru_token"))
	if err != nil && os.IsNotExist(err) {
		return errors.New("You're not logged in!")
	}
	fmt.Fprintln(context.Stdout, "Successfully logged out!")
	return nil
}

type teamCreate struct{}

func (c *teamCreate) Info() *Info {
	return &Info{
		Name:    "team-create",
		Usage:   "team-create <teamname>",
		Desc:    "creates a new team.",
		MinArgs: 1,
	}
}

func (c *teamCreate) Run(context *Context, client *Client) error {
	team := context.Args[0]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s"}`, team))
	url, err := GetURL("/teams")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `Team "%s" successfully created!`+"\n", team)
	return nil
}

type teamRemove struct{}

func (c *teamRemove) Run(context *Context, client *Client) error {
	team := context.Args[0]
	var answer string
	fmt.Fprintf(context.Stdout, `Are you sure you want to remove team "%s"? (y/n) `, team)
	fmt.Fscanf(context.Stdin, "%s", &answer)
	if answer != "y" {
		fmt.Fprintln(context.Stdout, "Abort.")
		return nil
	}
	url, err := GetURL(fmt.Sprintf("/teams/%s", team))
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
	fmt.Fprintf(context.Stdout, `Team "%s" successfully removed!`+"\n", team)
	return nil
}

func (c *teamRemove) Info() *Info {
	return &Info{
		Name:    "team-remove",
		Usage:   "team-remove <team-name>",
		Desc:    "removes a team from tsuru server.",
		MinArgs: 1,
	}
}

type teamUserAdd struct{}

func (c *teamUserAdd) Info() *Info {
	return &Info{
		Name:    "team-user-add",
		Usage:   "team-user-add <teamname> <useremail>",
		Desc:    "adds a user to a team.",
		MinArgs: 2,
	}
}

func (c *teamUserAdd) Run(context *Context, client *Client) error {
	teamName, userName := context.Args[0], context.Args[1]
	url, err := GetURL(fmt.Sprintf("/teams/%s/%s", teamName, userName))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, `User "%s" was added to the "%s" team`+"\n", userName, teamName)
	return nil
}

type teamUserRemove struct{}

func (c *teamUserRemove) Info() *Info {
	return &Info{
		Name:    "team-user-remove",
		Usage:   "team-user-remove <teamname> <useremail>",
		Desc:    "removes a user from a team.",
		MinArgs: 2,
	}
}

func (c *teamUserRemove) Run(context *Context, client *Client) error {
	teamName, userName := context.Args[0], context.Args[1]
	url, err := GetURL(fmt.Sprintf("/teams/%s/%s", teamName, userName))
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
	fmt.Fprintf(context.Stdout, `User "%s" was removed from the "%s" team`+"\n", userName, teamName)
	return nil
}

type teamUserList struct{}

func (teamUserList) Run(context *Context, client *Client) error {
	teamName := context.Args[0]
	url, err := GetURL("/teams/" + teamName)
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
	var t struct{ Users []string }
	err = json.NewDecoder(resp.Body).Decode(&t)
	if err != nil {
		return err
	}
	sort.Strings(t.Users)
	for _, user := range t.Users {
		fmt.Fprintf(context.Stdout, "- %s\n", user)
	}
	return nil
}

func (teamUserList) Info() *Info {
	return &Info{
		Name:    "team-user-list",
		Usage:   "team-user-list",
		Desc:    "List members of a team.",
		MinArgs: 1,
	}
}

type teamList struct{}

func (c *teamList) Info() *Info {
	return &Info{
		Name:    "team-list",
		Usage:   "team-list",
		Desc:    "List all teams that you are member.",
		MinArgs: 0,
	}
}

func (c *teamList) Run(context *Context, client *Client) error {
	url, err := GetURL("/teams")
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
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		var teams []map[string]string
		err = json.Unmarshal(b, &teams)
		if err != nil {
			return err
		}
		io.WriteString(context.Stdout, "Teams:\n\n")
		for _, team := range teams {
			fmt.Fprintf(context.Stdout, "  - %s\n", team["name"])
		}
	}
	return nil
}

type changePassword struct{}

func (c *changePassword) Run(context *Context, client *Client) error {
	url, err := GetURL("/users/password")
	if err != nil {
		return err
	}
	var body bytes.Buffer
	fmt.Fprint(context.Stdout, "Current password: ")
	old, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "\nNew password: ")
	new, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprint(context.Stdout, "\nConfirm: ")
	confirm, err := passwordFromReader(context.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout)
	if new != confirm {
		return errors.New("New password and password confirmation didn't match.")
	}
	jsonBody := map[string]string{
		"old": old,
		"new": new,
	}
	err = json.NewEncoder(&body).Encode(jsonBody)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, &body)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Password successfully updated!")
	return nil
}

func (c *changePassword) Info() *Info {
	return &Info{
		Name:  "change-password",
		Usage: "change-password",
		Desc:  "Change your password.",
	}
}

type resetPassword struct {
	token string
}

func (c *resetPassword) Info() *Info {
	return &Info{
		Name:  "reset-password",
		Usage: "reset-password <email> [--token|-t <token>]",
		Desc: `Redefines the user password.

This process is composed by two steps:

1. Generate a new token
2. Reset the password using the token

In order to generate the token, users should run this command without the --token flag.
The token will be mailed to the user.

With the token in hand, the user can finally reset the password using the --token flag.
The new password will also be mailed to the user.`,
		MinArgs: 1,
	}
}

func (c *resetPassword) msg() string {
	if c.token == "" {
		return `You've successfully started the password reset process.

Please check your email.`
	}
	return `Your password has been redefined and mailed to you.

Please check your email.`
}

func (c *resetPassword) Run(context *Context, client *Client) error {
	url := fmt.Sprintf("/users/%s/password", context.Args[0])
	if c.token != "" {
		url += "?token=" + c.token
	}
	url, err := GetURL(url)
	if err != nil {
		return err
	}
	request, _ := http.NewRequest("POST", url, nil)
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, c.msg())
	return nil
}

func (c *resetPassword) Flags() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("reset-password", gnuflag.ExitOnError)
	fs.StringVar(&c.token, "token", "", "Token to reset the password")
	fs.StringVar(&c.token, "t", "", "Token to reset the password")
	return fs
}

func passwordFromReader(reader io.Reader) (string, error) {
	var (
		password string
		err      error
	)
	if file, ok := reader.(*os.File); ok {
		password, err = term.ReadPassword(file.Fd())
		if err != nil {
			return "", err
		}
	} else {
		fmt.Fscanf(reader, "%s\n", &password)
	}
	if password == "" {
		msg := "You must provide the password!"
		return "", errors.New(msg)
	}
	return password, err
}

func scheme() string {
	info, err := schemeInfo()
	if err == nil {
		authScheme := info["name"]
		if authScheme != "" {
			return authScheme.(string)
		}
	}
	return "native"
}

func schemeInfo() (map[string]interface{}, error) {
	url, err := GetURL("/auth/scheme")
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	info := map[string]interface{}{}
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
