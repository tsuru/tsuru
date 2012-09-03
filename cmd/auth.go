package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/cmd/term"
	"io"
	"io/ioutil"
	"net/http"
	"os"
)

type passwordReader interface {
	readPassword(io.Writer, string) (string, error)
}

type stdinPasswordReader struct{}

func (r stdinPasswordReader) readPassword(out io.Writer, msg string) (string, error) {
	io.WriteString(out, msg)
	password, err := term.ReadPassword(os.Stdin.Fd())
	if err != nil {
		return "", err
	}
	io.WriteString(out, "\n")
	if password == "" {
		msg := "You must provide the password!\n"
		io.WriteString(out, msg)
		return "", errors.New(msg)
	}
	return password, nil
}

type userCreate struct {
	reader passwordReader
}

func (c *userCreate) preader() passwordReader {
	if c.reader == nil {
		c.reader = stdinPasswordReader{}
	}
	return c.reader
}

func (c *userCreate) Info() *Info {
	return &Info{
		Name:    "user-create",
		Usage:   "user-create <email>",
		Desc:    "creates a user.",
		MinArgs: 1,
	}
}

func (c *userCreate) Run(context *Context, client Doer) error {
	email := context.Args[0]
	password, err := c.preader().readPassword(context.Stdout, "Password: ")
	if err != nil {
		return err
	}
	confirm, err := c.preader().readPassword(context.Stdout, "Confirm: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("Passwords didn't match.")
	}
	b := bytes.NewBufferString(`{"email":"` + email + `", "password":"` + password + `"}`)
	request, err := http.NewRequest("POST", GetUrl("/users"), b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`User "%s" successfully created!`+"\n", email))
	return nil
}

type login struct {
	reader passwordReader
}

func (c *login) preader() passwordReader {
	if c.reader == nil {
		c.reader = stdinPasswordReader{}
	}
	return c.reader
}

func (c *login) Run(context *Context, client Doer) error {
	email := context.Args[0]
	password, err := c.preader().readPassword(context.Stdout, "Password: ")
	if err != nil {
		return err
	}
	b := bytes.NewBufferString(`{"password":"` + password + `"}`)
	request, err := http.NewRequest("POST", GetUrl("/users/"+email+"/tokens"), b)
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
	out := make(map[string]string)
	err = json.Unmarshal(result, &out)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Successfully logged!\n")
	return writeToken(out["token"])
}

func (c *login) Info() *Info {
	return &Info{
		Name:    "login",
		Usage:   "login <email>",
		Desc:    "log in with your credentials.",
		MinArgs: 1,
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

func (c *logout) Run(context *Context, client Doer) error {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return err
	}
	err = filesystem().Remove(tokenPath)
	if err != nil && os.IsNotExist(err) {
		return errors.New("You're not logged in!")
	}
	io.WriteString(context.Stdout, "Successfully logout!\n")
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

func (c *teamCreate) Run(context *Context, client Doer) error {
	team := context.Args[0]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s"}`, team))
	request, err := http.NewRequest("POST", GetUrl("/teams"), b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`Team "%s" successfully created!`+"\n", team))
	return nil
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

func (c *teamUserAdd) Run(context *Context, client Doer) error {
	teamName, userName := context.Args[0], context.Args[1]
	url := GetUrl(fmt.Sprintf("/teams/%s/%s", teamName, userName))
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`User "%s" was added to the "%s" team`+"\n", userName, teamName))
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

func (c *teamUserRemove) Run(context *Context, client Doer) error {
	teamName, userName := context.Args[0], context.Args[1]
	url := GetUrl(fmt.Sprintf("/teams/%s/%s", teamName, userName))
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`User "%s" was removed from the "%s" team`+"\n", userName, teamName))
	return nil
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

func (c *teamList) Run(context *Context, client Doer) error {
	request, err := http.NewRequest("GET", GetUrl("/teams"), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
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
