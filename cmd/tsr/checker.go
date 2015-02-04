// Copyright 2014 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/tsuru/config"
)

func CheckBasicConfig() error {
	return checkConfigPresent([]string{
		"listen",
		"host",
		"database:url",
		"database:name",
		"git:api-server",
	}, "Config Error: you should have %q key set in your config file")
}

// Check provisioner configs
func CheckProvisioner() error {
	if value, _ := config.Get("provisioner"); value == "docker" || value == "" {
		return CheckDocker()
	}
	return nil
}

func CheckBeanstalkd() error {
	if value, _ := config.Get("queue"); value == "beanstalkd" {
		return errors.New("beanstalkd is no longer supported, please use redis instead")
	}
	if _, err := config.Get("queue-server"); err == nil {
		return errors.New(`beanstalkd is no longer supported, please remove the "queue-server" setting from your config file`)
	}
	return nil
}

// Check Docker configs
func CheckDocker() error {
	if _, err := config.Get("docker"); err != nil {
		return errors.New("Config Error: you should configure docker.")
	}
	err := CheckDockerBasicConfig()
	if err != nil {
		return err
	}
	err = CheckScheduler()
	if err != nil {
		return err
	}
	err = CheckRouter()
	if err != nil {
		return err
	}
	return checkCluster()
}

// Check default configs to Docker.
func CheckDockerBasicConfig() error {
	return checkConfigPresent([]string{
		"docker:repository-namespace",
		"docker:collection",
		"docker:deploy-cmd",
		"docker:ssh",
		"docker:ssh:user",
		"docker:run-cmd:bin",
		"docker:run-cmd:port",
	}, "Config Error: you should configure %q")
}

func checkCluster() error {
	storage, _ := config.GetString("docker:cluster:storage")
	if storage != "mongodb" && storage != "" {
		return fmt.Errorf("Config Error: docker:cluster:storage is deprecated. mongodb is now the only storage available.")
	}
	mustHave := []string{"docker:cluster:mongo-url", "docker:cluster:mongo-database"}
	for _, value := range mustHave {
		if _, err := config.Get(value); err != nil {
			return fmt.Errorf("Config Error: you should configure %q", value)
		}
	}
	return nil
}

// Check Schedulers
// It's verify your scheduler configuration and validate related confs.
func CheckScheduler() error {
	if scheduler, err := config.Get("docker:segregate"); err == nil && scheduler == true {
		if servers, err := config.Get("docker:servers"); err == nil && servers != nil {
			return fmt.Errorf("Your scheduler is the segregate. Please remove the servers conf in docker.")
		}
		return nil
	}
	return nil
}

// Check Router
// It's verify your router configuration and validate related confs.
func CheckRouter() error {
	if router, err := config.Get("docker:router"); err == nil && router == "hipache" {
		if hipache, err := config.Get("hipache"); err != nil || hipache == nil {
			return fmt.Errorf("You should configure hipache router")
		}
	}
	return nil
}

func checkConfigPresent(keys []string, fmtMsg string) error {
	for _, key := range keys {
		if _, err := config.Get(key); err != nil {
			return fmt.Errorf(fmtMsg, key)
		}
	}
	return nil
}
