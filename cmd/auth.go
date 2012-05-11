package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	return &Info{Name: "create-user"}
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

type LoginCommand struct{}

func (c *LoginCommand) Run(context *Context, client Doer) error {
	email, password := context.Args[0], context.Args[1]
	b := bytes.NewBufferString(`{"password":"` + password + `"}`)
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/users/"+email+"/tokens", b)
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
	io.WriteString(context.Stdout, "Successfully logged!")
	WriteToken(out["token"])
	return nil
}

func (c *LoginCommand) Info() *Info {
	return &Info{Name: "login"}
}
