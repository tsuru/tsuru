package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"strings"
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

func readKey() (string, error) {
	user, err := user.Current()
	keyPath := user.HomeDir + "/.ssh/id_rsa.pub"
	output, err := ioutil.ReadFile(keyPath)
	return string(output), err
}

type Key struct{}

func (c *Key) Info() *Info {
	return &Info{Name: "key"}
}

func (c *Key) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"add":    &AddKeyCommand{},
		"remove": &RemoveKey{},
	}
}

type RemoveKey struct{}

func (c *RemoveKey) Info() *Info {
	return &Info{Name: "remove"}
}

func (c *RemoveKey) Run(context *Context, client Doer) error {
	key, err := readKey()
	if os.IsNotExist(err) {
		io.WriteString(context.Stderr, "You don't have a public key\n")
		io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
		return nil
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("DELETE", "http://tsuru.plataformas.glb.com:8080/users/keys", b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Key removed with success!\n")
	return nil
}

type AddKeyCommand struct{}

func (c *AddKeyCommand) Info() *Info {
	return &Info{Name: "add"}
}

func (c *AddKeyCommand) Run(context *Context, client Doer) error {
	key, err := readKey()
	if os.IsNotExist(err) {
		io.WriteString(context.Stderr, "You don't have a public key\n")
		io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
		return nil
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/users/keys", b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Key added with success!\n")
	return nil
}

type LogoutCommand struct{}

func (c *LogoutCommand) Info() *Info {
	return &Info{Name: "logout"}
}

func (c *LogoutCommand) Run(context *Context, client Doer) error {
	err := WriteToken("")
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, "Successfully logout!\n")
	return nil
}

type Team struct{}

func (c *Team) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"add-user":    &TeamAddUser{},
		"remove-user": &TeamRemoveUser{},
		"create":      &TeamCreate{},
	}
}

func (c *Team) Info() *Info {
	return &Info{Name: "team"}
}

func (c *Team) Run(context *Context, client Doer) error {
	return nil
}

type TeamCreate struct{}

func (c *TeamCreate) Info() *Info {
	return &Info{Name: "create"}
}

func (c *TeamCreate) Run(context *Context, client Doer) error {
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

type TeamAddUser struct{}

func (c *TeamAddUser) Info() *Info {
	return &Info{Name: "add-user"}
}

func (c *TeamAddUser) Run(context *Context, client Doer) error {
	teamName, userName := context.Args[0], context.Args[1]
	request, err := http.NewRequest("PUT", fmt.Sprintf("http://tsuru.plataformas.glb.com:8080/teams/%s/%s", teamName, userName), nil)
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

type TeamRemoveUser struct{}

func (c *TeamRemoveUser) Info() *Info {
	return &Info{Name: "remove-user"}
}

func (c *TeamRemoveUser) Run(context *Context, client Doer) error {
	teamName, userName := context.Args[0], context.Args[1]
	request, err := http.NewRequest("DELETE", fmt.Sprintf("http://tsuru.plataformas.glb.com:8080/teams/%s/%s", teamName, userName), nil)
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
