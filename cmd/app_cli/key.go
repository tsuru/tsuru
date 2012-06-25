package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

func readKey() (string, error) {
	user, err := user.Current()
	keyPath := user.HomeDir + "/.ssh/id_rsa.pub"
	output, err := ioutil.ReadFile(keyPath)
	return string(output), err
}

type Key struct{}

func (c *Key) Info() *Info {
	return &Info{
		Name:    "key",
		Usage:   "key (add|remove)",
		Desc:    "manage keys.",
		MinArgs: 1,
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

func (c *RemoveKey) Run(context *cmd.Context, client Doer) error {
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
	io.WriteString(context.Stdout, "Key successfully removed!\n")
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

func (c *AddKeyCommand) Run(context *cmd.Context, client Doer) error {
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
	io.WriteString(context.Stdout, "Key successfully added!\n")
	return nil
}
