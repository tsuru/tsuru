package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
)

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
