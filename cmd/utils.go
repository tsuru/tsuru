package main

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
)

func joinWithUserDir(p ...string) (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	paths := []string{user.HomeDir}
	paths = append(paths, p...)
	return path.Join(paths...), nil
}

func WriteToken(token string) error {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return err
	}
	file, err := os.Create(tokenPath)
	if err != nil {
		return err
	}
	_, err = file.WriteString(token)
	if err != nil {
		return err
	}
	return nil
}

func ReadToken() (string, error) {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return "", err
	}
	token, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
