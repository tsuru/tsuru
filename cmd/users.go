package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

type AddUserCommand struct{}

func (c *AddUserCommand) Run(context *Context, client Doer) error {
	email, password := context.Args[0], context.Args[1]
	b := bytes.NewBufferString(`{"email":"` + email + `", "password":"` + password + `"}`)
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/users", b)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Creating new user: "+email+"\n")
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "OK")
	return nil
}

func (c *AddUserCommand) Info() *Info {
	return &Info{Name: "add-user"}
}

type CreateTeamCommand struct{}

func (c *CreateTeamCommand) Info() *Info {
	return &Info{Name: "create-team"}
}

func (c *CreateTeamCommand) Run(context *Context, client Doer) error {
	team := context.Args[0]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s"}`, team))
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/teams", b)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf("Creating new team: %s\n", team))
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "OK")
	return nil
}
