// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nginx

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"os/exec"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

func AddRoute(name, ip string) error {
	domain, err := config.GetString("nginx:domain")
	if err != nil {
		return err
	}
	routesPath, err := config.GetString("nginx:routes-path")
	if err != nil {
		return err
	}
	file, _ := filesystem().Create(routesPath + "/" + name)
	defer file.Close()
	template := `server {
	listen 80;
	%s.%s;
	location / {
		proxy_pass http://%s;
	}
}`
	template = fmt.Sprintf(template, name, domain, ip)
	data := []byte(template)
	_, err = file.Write(data)
	return err
}

func RestartRouter() error {
	cmd := exec.Command("sudo", "service", "nginx", "restart")
	return cmd.Run()
}
