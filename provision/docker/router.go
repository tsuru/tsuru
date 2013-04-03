// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/log"
	"os/exec"
)

func AddRoute(name, ip string) error {
	domain, err := config.GetString("docker:domain")
	if err != nil {
		return err
	}
	routesPath, err := config.GetString("docker:routes-path")
	if err != nil {
		return err
	}
	filename := routesPath + "/" + name
	template := `server { listen 80; %s.%s; location / { proxy_pass http://%s; } }`
	template = fmt.Sprintf(template, name, domain, ip)
	err, _ = runCmd("sudo", "echo", "\"", template, "\"", "|", "sudo tee", filename)
	if err != nil {
		log.Printf("error(%s) trying to write file: %s", filename)
	}
	return err
}

func RestartRouter() error {
	cmd := exec.Command("sudo", "service", "nginx", "restart")
	return cmd.Run()
}
