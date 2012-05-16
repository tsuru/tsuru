package cmd

import (
	"io/ioutil"
	"os"
	"os/user"
)

func WriteToken(token string) error {
	user, err := user.Current()
	tokenPath := user.HomeDir + "/.tsuru_token"
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
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenPath := user.HomeDir + "/.tsuru_token"
	token, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
