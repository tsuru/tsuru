// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nginx

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/router"
	"os/exec"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

func init() {
	router.Register("nginx", &NginxRouter{})
}

type NginxRouter struct{}

func (r *NginxRouter) AddRoute(name, address string) error {
	domain, err := config.GetString("nginx:domain")
	if err != nil {
		return err
	}
	routesPath, err := config.GetString("nginx:routes-path")
	if err != nil {
		return err
	}
	file, err := filesystem().Create(routesPath + "/" + name)
	if err != nil {
		return err
	}
	defer file.Close()
	template := `server {
	listen 80;
	server_name %s.%s;
	location / {
		proxy_pass http://%s;
	}
}`
	template = fmt.Sprintf(template, name, domain, address)
	data := []byte(template)
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return r.restart()
}

func (r *NginxRouter) RemoveRoute(name, address string) error {
	return nil
}

func (NginxRouter) SetCName(cname, name string) error {
	return nil
}

func (NginxRouter) UnsetCName(cname string) error {
	return nil
}

func (r *NginxRouter) AddBackend(name string) error {
	return nil
}

func (r *NginxRouter) RemoveBackend(name string) error {
	routesPath, err := config.GetString("nginx:routes-path")
	if err != nil {
		return err
	}
	err = filesystem().Remove(routesPath + "/" + name)
	if err != nil {
		return err
	}
	return r.restart()
}

func (NginxRouter) restart() error {
	cmd := exec.Command("sudo", "service", "nginx", "restart")
	return cmd.Run()
}

func (NginxRouter) Addr(name string) (string, error) {
	domain, _ := config.GetString("nginx:domain")
	return fmt.Sprintf("%s.%s", name, domain), nil
}
