package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/cmd"
	"github.com/timeredbull/tsuru/fs"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"strings"
)

type keyReader struct {
	fsystem fs.Fs
}

func (r *keyReader) fs() fs.Fs {
	if r.fsystem == nil {
		r.fsystem = fs.OsFs{}
	}
	return r.fsystem
}

func getKeyPath(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	return user.HomeDir + "/.ssh/id_rsa.pub", nil
}

func (r *keyReader) readKey(keyPath string) (string, error) {
	f, err := r.fs().Open(keyPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	output, err := ioutil.ReadAll(f)
	return string(output), err
}

type Key struct{}

func (c *Key) Info() *cmd.Info {
	return &cmd.Info{
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

type RemoveKey struct {
	keyReader
}

func (c *RemoveKey) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "remove",
		Usage: "key remove [path/to/key/file.pub]",
		Desc:  "remove your public key ($HOME/.id_rsa.pub by default).",
	}
}

func (c *RemoveKey) Run(context *cmd.Context, client cmd.Doer) error {
	keyPath, err := getKeyPath(context.Args)
	if err != nil {
		return err
	}
	key, err := c.readKey(keyPath)
	if os.IsNotExist(err) {
		io.WriteString(context.Stderr, "You don't have a public key\n")
		io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
		return errors.New("You need to have a public rsa key")
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("DELETE", cmd.GetUrl("/users/keys"), b)
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

type AddKeyCommand struct {
	keyReader
}

func (c *AddKeyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "add",
		Usage: "key add [path/to/key/file.pub]",
		Desc:  "add your public key ($HOME/.ssh/id_rsa.pub by default).",
	}
}

func (c *AddKeyCommand) Run(context *cmd.Context, client cmd.Doer) error {
	keyPath, err := getKeyPath(context.Args)
	if err != nil {
		return err
	}
	key, err := c.readKey(keyPath)
	if os.IsNotExist(err) {
		if len(context.Args) > 0 {
			msg := fmt.Sprintf("File %s does not exist!", keyPath)
			io.WriteString(context.Stderr, msg)
			return errors.New(msg)
		} else {
			io.WriteString(context.Stderr, "You don't have a public key\n")
			io.WriteString(context.Stderr, "To generate a key use 'ssh-keygen' command\n")
			return errors.New("You need to have a public rsa key")
		}
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	request, err := http.NewRequest("POST", cmd.GetUrl("/users/keys"), b)
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
