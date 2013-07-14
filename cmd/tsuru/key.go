// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"github.com/globocom/tsuru/fs"
	"io/ioutil"
	"net/http"
	"os"
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
	home := os.ExpandEnv("$HOME")
	return home + "/.ssh/id_rsa.pub", nil
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

func (r *keyReader) fileNotFound(context *cmd.Context) error {
	if len(context.Args) > 0 {
		msg := fmt.Sprintf("File %s does not exist!", context.Args[0])
		fmt.Fprint(context.Stderr, msg+"\n")
		return errors.New(msg)
	}
	msg := "You don't have a public key\nTo generate a key use 'ssh-keygen' command\n"
	fmt.Fprint(context.Stderr, msg)
	return errors.New("You need to have a public rsa key")
}

type KeyRemove struct {
	keyReader
}

func (c *KeyRemove) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "key-remove",
		Usage: "key-remove [path/to/key/file.pub]",
		Desc:  "remove your public key ($HOME/.id_rsa.pub by default).",
	}
}

func (c *KeyRemove) Run(context *cmd.Context, client *cmd.Client) error {
	keyPath, err := getKeyPath(context.Args)
	if err != nil {
		return err
	}
	key, err := c.readKey(keyPath)
	if os.IsNotExist(err) {
		return c.fileNotFound(context)
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	url, err := cmd.GetURL("/users/keys")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("DELETE", url, b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Key %q successfully removed!\n", keyPath)
	return nil
}

type KeyAdd struct {
	keyReader
}

func (c *KeyAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "key-add",
		Usage: "key-add [path/to/key/file.pub]",
		Desc:  "add your public key ($HOME/.ssh/id_rsa.pub by default).",
	}
}

func (c *KeyAdd) Run(context *cmd.Context, client *cmd.Client) error {
	keyPath, err := getKeyPath(context.Args)
	if err != nil {
		return err
	}
	key, err := c.readKey(keyPath)
	if os.IsNotExist(err) {
		return c.fileNotFound(context)
	}
	b := bytes.NewBufferString(fmt.Sprintf(`{"key":"%s"}`, strings.Replace(key, "\n", "", -1)))
	url, err := cmd.GetURL("/users/keys")
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Key %q successfully added!\n", keyPath)
	return nil
}
