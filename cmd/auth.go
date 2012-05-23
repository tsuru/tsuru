package cmd

//#include <stdlib.h>
//#include "pass.h"
import "C"

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"strings"
	"unsafe"
)

type User struct{}

func getPassword(fd uintptr) string {
	cPasswd := C.GetPassword(C.int(fd))
	defer C.free(unsafe.Pointer(cPasswd))
	return C.GoString(cPasswd)
}

func readPassword(out io.Writer, password *string) error {
	io.WriteString(out, "Password: ")
	*password = getPassword(os.Stdin.Fd())
	io.WriteString(out, "\n")
	if *password == "" {
		msg := "You must provide the password!\n"
		io.WriteString(out, msg)
		return errors.New(msg)
	}
	return nil
}

func (c *User) Info() *Info {
	return &Info{
		Name:  "user",
		Usage: "user (create) [args]",
		Desc:  "manage users.",
		Args:  1,
	}
}

func (c *User) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"create": &UserCreate{},
	}
}

type UserCreate struct{}

func (c *UserCreate) Info() *Info {
	return &Info{
		Name:  "create",
		Usage: "user create username password",
		Desc:  "creates user.",
		Args:  1,
	}
}

func (c *UserCreate) Run(context *Context, client Doer) error {
	var password string
	email := context.Args[0]
	err := readPassword(context.Stdout, &password)
	if err != nil {
		return err
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
	io.WriteString(context.Stdout, fmt.Sprintf(`User "%s" created with success!`+"\n", email))
	return nil
}

type Login struct{}

func (c *Login) Run(context *Context, client Doer) error {
	var password string
	email := context.Args[0]
	err := readPassword(context.Stdout, &password)
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
	WriteToken(out["token"])
	return nil
}

func (c *Login) Info() *Info {
	return &Info{
		Name:  "login",
		Usage: "login email",
		Desc:  "log in with your credentials.",
		Args:  1,
	}
}

func readKey() (string, error) {
	user, err := user.Current()
	keyPath := user.HomeDir + "/.ssh/id_rsa.pub"
	output, err := ioutil.ReadFile(keyPath)
	return string(output), err
}

type Key struct{}

func (c *Key) Info() *Info {
	return &Info{
		Name:  "key",
		Usage: "key (add|remove)",
		Desc:  "manage keys.",
		Args:  1,
	}
}

func (c *Key) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"add":    &AddKeyCommand{},
		"remove": &RemoveKey{},
	}
}

type RemoveKey struct{}

func (c *RemoveKey) Info() *Info {
	return &Info{
		Name:  "remove",
		Usage: "key remove",
		Desc:  "remove your public key ($HOME/.id_rsa.pub).",
	}
}

func (c *RemoveKey) Run(context *Context, client Doer) error {
	key, err := readKey()
	if os.IsNotExist(err) {
		io.WriteString(context.Stderr, "You don't have a public key\n")
		io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
		return nil
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("DELETE", GetUrl("/users/keys"), b)
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
	return &Info{
		Name:  "add",
		Usage: "key add",
		Desc:  "add your public key ($HOME/.id_rsa.pub).",
	}
}

func (c *AddKeyCommand) Run(context *Context, client Doer) error {
	key, err := readKey()
	if os.IsNotExist(err) {
		io.WriteString(context.Stderr, "You don't have a public key\n")
		io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
		return nil
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("POST", GetUrl("/users/keys"), b)
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

type Logout struct{}

func (c *Logout) Info() *Info {
	return &Info{
		Name:  "logout",
		Usage: "logout",
		Desc:  "clear local authentication credentials.",
	}
}

func (c *Logout) Run(context *Context, client Doer) error {
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
	return &Info{
		Name:  "team",
		Usage: "team (create|add-user|remove-user) [args]",
		Desc:  "manage teams.",
		Args:  1,
	}
}

type TeamCreate struct{}

func (c *TeamCreate) Info() *Info {
	return &Info{
		Name:  "create",
		Usage: "team create teamname",
		Desc:  "creates teams.",
		Args:  1,
	}
}

func (c *TeamCreate) Run(context *Context, client Doer) error {
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
	io.WriteString(context.Stdout, fmt.Sprintf(`Team "%s" created with success!`+"\n", team))
	return nil
}

type TeamAddUser struct{}

func (c *TeamAddUser) Info() *Info {
	return &Info{
		Name:  "add-user",
		Usage: "glb team add-user teamname username",
		Desc:  "adds user to a team",
		Args:  2,
	}
}

func (c *TeamAddUser) Run(context *Context, client Doer) error {
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

type TeamRemoveUser struct{}

func (c *TeamRemoveUser) Info() *Info {
	return &Info{
		Name:  "remove-user",
		Usage: "glb team remove-user teamname username",
		Desc:  "removes user from a team",
		Args:  2,
	}
}

func (c *TeamRemoveUser) Run(context *Context, client Doer) error {
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
