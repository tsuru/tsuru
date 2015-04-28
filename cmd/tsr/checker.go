// Copyright 2015 Globo.com. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/tsuru/config"
)

func checkBasicConfig() error {
	return checkConfigPresent([]string{
		"listen",
		"host",
		"database:url",
		"database:name",
	}, "Config Error: you should have %q key set in your config file")
}

func checkGandalf() error {
	if value, err := config.GetString("repo-manager"); value == "gandalf" || value == "" {
		_, err = config.Get("git:api-server")
		if err != nil {
			return fmt.Errorf("config error: you must define the %q config key", "git:api-server")
		}
	}
	return nil
}

// Check provisioner configs
func checkProvisioner() error {
	if value, _ := config.Get("provisioner"); value == "docker" || value == "" {
		return checkDocker()
	}
	return nil
}

func checkBeanstalkd() error {
	if value, _ := config.Get("queue"); value == "beanstalkd" {
		return errors.New("beanstalkd is no longer supported, please use redis instead")
	}
	if _, err := config.Get("queue-server"); err == nil {
		return errors.New(`beanstalkd is no longer supported, please remove the "queue-server" setting from your config file`)
	}
	return nil
}

func checkPubSub() error {
	oldConfig, _ := config.GetString("redis-queue:host")
	if oldConfig != "" {
		return config.NewWarning(`Using "redis-queue:*" is deprecated. Please change your tsuru.conf to use "pubsub:*" options. See http://docs.tsuru.io/en/latest/reference/config.html#pubsub for more details.`)
	}
	redisHost, _ := config.GetString("pubsub:redis-host")
	if redisHost == "" {
		return config.NewWarning(`Config entry "pubsub:redis-host" is not set, default "localhost" will be used. Running "tsuru app-log -f" might not work.`)
	}
	return nil
}

func checkQueue() error {
	queueConfig, _ := config.GetString("queue:mongo-url")
	if queueConfig == "" {
		return config.NewWarning(`Config entry "queue:mongo-url" is not set, default "localhost" will be used. Running "tsuru-admin docker-node-{add,remove}" commands might not work.`)
	}
	return nil
}

// Check Docker configs
func checkDocker() error {
	if _, err := config.Get("docker"); err != nil {
		return errors.New("Config Error: you should configure docker.")
	}
	err := checkDockerBasicConfig()
	if err != nil {
		return err
	}
	err = checkScheduler()
	if err != nil {
		return err
	}
	err = checkRouter()
	if err != nil {
		return err
	}
	return checkCluster()
}

// Check default configs to Docker.
func checkDockerBasicConfig() error {
	return checkConfigPresent([]string{
		"docker:repository-namespace",
		"docker:collection",
		"docker:deploy-cmd",
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
// It verifies your scheduler configuration and validates related confs.
func checkScheduler() error {
	if scheduler, err := config.Get("docker:segregate"); err == nil && scheduler == true {
		if servers, err := config.Get("docker:servers"); err == nil && servers != nil {
			return fmt.Errorf("Your scheduler is the segregate. Please remove the servers conf in docker.")
		}
		return nil
	}
	return nil
}

// Check Router
// It verifies your router configuration and validates related confs.
func checkRouter() error {
	defaultRouter, _ := config.GetString("docker:router")
	if defaultRouter == "" {
		return fmt.Errorf(`You must configure a default router in "docker:router".`)
	}
	isHipacheOld := false
	if defaultRouter == "hipache" {
		hipacheOld, _ := config.Get("hipache")
		isHipacheOld = hipacheOld != nil
	}
	routerConf, _ := config.Get("routers:" + defaultRouter)
	if isHipacheOld {
		return config.NewWarning(`Setting "hipache:*" config entries is deprecated. You should configure your router with "routers:*". See http://docs.tsuru.io/en/latest/reference/config.html#routers for more details.`)
	}
	if routerConf == nil {
		return fmt.Errorf(`You must configure your default router %q in "routers:%s".`, defaultRouter, defaultRouter)
	}
	routerType, _ := config.Get("routers:" + defaultRouter + ":type")
	if routerType == nil {
		return fmt.Errorf(`You must configure your default router type in "routers:%s:type".`, defaultRouter)
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
