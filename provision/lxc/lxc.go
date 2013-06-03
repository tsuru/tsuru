// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lxc

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"io/ioutil"
	"net"
	"runtime"
	"strings"
	"time"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// container represents an lxc container with the given name.
type container struct {
	name string
	ip   string
}

// runCmd executes commands and log the given stdout and stderror.
func runCmd(cmd string, args ...string) error {
	output := bytes.Buffer{}
	err := executor().Execute(cmd, args, nil, &output, &output)
	log.Printf("running the cmd: %s with the args: %s", cmd, args)
	log.Print(output.String())
	return err
}

// Ip returns the ip for the container.
func (c *container) IP() string {
	if c.ip != "" {
		return c.ip
	}
	timeout, err := config.GetInt("lxc:ip-timeout")
	if err != nil {
		timeout = 60
	}
	quit := time.After(time.Duration(timeout) * time.Second)
	tick := time.Tick(2 * time.Second)
	stop := false
	for !stop {
		select {
		case <-tick:
			file, _ := filesystem().Open("/var/lib/misc/dnsmasq.leases")
			data, _ := ioutil.ReadAll(file)
			log.Print("dnsmasq.leases")
			log.Print(string(data))
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Index(line, c.name) != -1 {
					log.Printf("ip in %s", line)
					c.ip = strings.Split(line, " ")[2]
					return c.ip
				}
			}
		case <-quit:
			stop = true
		default:
			time.Sleep(1 * time.Second)
		}
	}
	return ""
}

// create creates a lxc container with ubuntu template by default.
func (c *container) create() error {
	keyPath, err := config.GetString("lxc:authorized-key-path")
	if err != nil {
		return err
	}
	return runCmd("sudo", "lxc-create", "-t", "ubuntu-cloud", "-n", c.name, "--", "-S", keyPath)
}

// start starts a lxc container.
func (c *container) start() error {
	return runCmd("sudo", "lxc-start", "--daemon", "-n", c.name)
}

// stop stops a lxc container.
func (c *container) stop() error {
	return runCmd("sudo", "lxc-stop", "-n", c.name)
}

// destroy destory a lxc container.
func (c *container) destroy() error {
	return runCmd("sudo", "lxc-destroy", "-n", c.name)
}

// waitForNetwork waits the container network is up.
func (c *container) waitForNetwork() error {
	t, err := config.GetInt("lxc:ip-timeout")
	if err != nil {
		t = 60
	}
	timeout := time.After(time.Duration(t) * time.Second)
	done := make(chan bool, 1)
	go func(c *container) {
		for {
			port, err := config.GetInt("lxc:ssh-port")
			if err != nil {
				port = 22
			}
			addr := fmt.Sprintf("%s:%d", c.IP(), port)
			conn, err := net.Dial("tcp", addr)
			if err == nil {
				conn.Close()
				done <- true
				break
			}
			runtime.Gosched()
		}
	}(c)
	select {
	case <-done:
	case <-timeout:
		return errors.New("timeout")
	}
	return nil
}
