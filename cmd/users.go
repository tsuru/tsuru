package cmd

import (
	"bytes"
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
