// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/pkg/errors"
	"github.com/tsuru/config"
)

func checkBasicConfig() error {
	return checkConfigPresent([]string{
		"listen",
		"host",
	}, "Config error: you should have %q key set in your config file")
}

func checkDatabase() error {
	if value, _ := config.GetString("database:driver"); value != "mongodb" && value != "" {
		return errors.Errorf("Config error: mongodb is the only database driver currently supported")
	}
	return checkConfigPresent([]string{
		"database:url",
		"database:name",
	}, "Config error: you should have %q key set in your config file")
}

// Check provisioner configs
func checkProvisioner() error {
	if value, _ := config.Get("provisioner"); value == defaultProvisionerName || value == "" {
		return checkDocker()
	}
	return nil
}

// Check Docker configs
func checkDocker() error {
	if _, err := config.Get("docker"); err != nil {
		return errors.New("Config error: you should configure docker.")
	}
	err := checkDockerBasicConfig()
	if err != nil {
		return err
	}
	return checkCluster()
}

// Check default configs to Docker.
func checkDockerBasicConfig() error {
	return checkConfigPresent([]string{
		"docker:collection",
	}, "Config error: you should configure %q")
}

func checkCluster() error {
	storage, _ := config.GetString("docker:cluster:storage")
	if storage != "mongodb" && storage != "" {
		return errors.Errorf("Config error: docker:cluster:storage is deprecated. mongodb is now the only storage available.")
	}
	mustHave := []string{"docker:cluster:mongo-url", "docker:cluster:mongo-database"}
	for _, value := range mustHave {
		if _, err := config.Get(value); err != nil {
			return errors.Errorf("Config error: you should configure %q", value)
		}
	}
	return nil
}

func checkConfigPresent(keys []string, fmtMsg string) error {
	for _, key := range keys {
		if _, err := config.Get(key); err != nil {
			return errors.Errorf(fmtMsg, key)
		}
	}
	return nil
}
