// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os/user"
	"path"
	"strings"
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

func writeToken(token string) error {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return err
	}
	file, err := filesystem().Create(tokenPath)
	if err != nil {
		return err
	}
	n, err := file.WriteString(token)
	if err != nil {
		return err
	}
	if n != len(token) {
		return errors.New("Failed to write token file.")
	}
	return nil
}

func readToken() (string, error) {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return "", err
	}
	file, err := filesystem().Open(tokenPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	token, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(token), nil
}

type ServiceModel struct {
	Service   string
	Instances []string
}

func ShowServicesInstancesList(b []byte) ([]byte, error) {
	var services []ServiceModel
	err := json.Unmarshal(b, &services)
	if err != nil {
		return []byte{}, err
	}
	if len(services) == 0 {
		return []byte{}, nil
	}
	table := NewTable()
	table.Headers = Row([]string{"Services", "Instances"})
	for _, s := range services {
		insts := strings.Join(s.Instances, ", ")
		r := Row([]string{s.Service, insts})
		table.AddRow(r)
	}
	return table.Bytes(), nil
}
