package cmd

import (
	"bytes"
	"io"
	"net/http"
)

type LoginCommand struct{}

func (c *LoginCommand) Run(context *Context, client Doer) error {
	email, password := context.Args[0], context.Args[1]
	b := bytes.NewBufferString(`{"password":"` + password + `"}`)
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/"+email+"/tokens", b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Successfully logged!")
	return nil
}

func (c *LoginCommand) Info() *Info {
	return &Info{Name: "login"}
}
