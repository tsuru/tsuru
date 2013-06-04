// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lxc

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/deploy"
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/router"
	_ "github.com/globocom/tsuru/router/nginx"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

func init() {
	provision.Register("lxc", &LXCProvisioner{})
}

var execut exec.Executor

func executor() exec.Executor {
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

type LXCProvisioner struct{}

func (p *LXCProvisioner) router() (router.Router, error) {
	r, err := config.GetString("router")
	if err != nil {
		return nil, err
	}
	return router.Get(r)
}

func (p *LXCProvisioner) setup(ip, framework string) error {
	formulasPath, err := config.GetString("lxc:formulas-path")
	if err != nil {
		return err
	}
	log.Printf("Creating hooks dir for %s", ip)
	output := bytes.Buffer{}
	cmd := "ssh"
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo mkdir -p /var/lib/tsuru/hooks"}
	err = executor().Execute(cmd, args, nil, &output, &output)
	if err != nil {
		log.Printf("error on creating hooks dir for %s", ip)
		log.Print(output.String())
		log.Print(err)
		return err
	}
	log.Printf("Permissons on hooks dir for %s", ip)
	output = bytes.Buffer{}
	args = []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo chown -R ubuntu /var/lib/tsuru/hooks"}
	err = executor().Execute(cmd, args, nil, &output, &output)
	if err != nil {
		log.Printf("error on permissions for %s", ip)
		log.Print(output.String())
		log.Print(err)
		return err
	}
	log.Printf("coping hooks to %s", ip)
	output = bytes.Buffer{}
	cmd = "scp"
	args = []string{"-q", "-o", "StrictHostKeyChecking no", "-r", formulasPath + "/" + framework + "/hooks", "ubuntu@" + ip + ":/var/lib/tsuru"}
	err = executor().Execute(cmd, args, nil, &output, &output)
	if err != nil {
		log.Printf("error on execute scp with the args: %#v", args)
		log.Print(output.String())
		log.Print(err)
		return err
	}
	return nil
}

func (p *LXCProvisioner) install(ip string) error {
	log.Printf("executing the install hook for %s", ip)
	cmd := "ssh"
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/install"}
	err := executor().Execute(cmd, args, nil, nil, nil)
	if err != nil {
		log.Printf("error on install for %s", ip)
		log.Print(err)
		return err
	}
	return nil
}

func (p *LXCProvisioner) start(ip string) error {
	cmd := "ssh"
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/start"}
	return executor().Execute(cmd, args, nil, nil, nil)
}

func (p *LXCProvisioner) Provision(app provision.App) error {
	go func(p *LXCProvisioner, app provision.App) {
		c := container{name: app.GetName()}
		log.Printf("creating container %s", c.name)
		u := provision.Unit{
			Name:       app.GetName(),
			AppName:    app.GetName(),
			Type:       app.GetPlatform(),
			Machine:    0,
			InstanceId: app.GetName(),
			Status:     provision.StatusCreating,
			Ip:         "",
		}
		log.Printf("inserting container unit %s in the database", app.GetName())
		err := p.collection().Insert(u)
		if err != nil {
			log.Print(err)
		}
		err = c.create()
		if err != nil {
			log.Printf("error on create container %s", app.GetName())
			log.Print(err)
		}
		err = c.start()
		if err != nil {
			log.Printf("error on start container %s", app.GetName())
			log.Print(err)
		}
		u.Ip = c.IP()
		u.Status = provision.StatusInstalling
		err = p.collection().Update(bson.M{"name": u.Name}, u)
		if err != nil {
			log.Print(err)
		}
		err = c.waitForNetwork()
		if err != nil {
			log.Print(err)
		}
		err = p.setup(c.IP(), app.GetPlatform())
		if err != nil {
			log.Printf("error on setup container %s", app.GetName())
			log.Print(err)
		}
		err = p.install(c.IP())
		if err != nil {
			log.Printf("error on install container %s", app.GetName())
			log.Print(err)
		}
		err = p.start(c.IP())
		if err != nil {
			log.Printf("error on start app for container %s", app.GetName())
			log.Print(err)
		}
		err = p.start(c.IP())
		r, err := p.router()
		if err != nil {
			log.Print(err)
			return
		}
		err = r.AddBackend(app.GetName())
		if err != nil {
			log.Printf("error on add backend for %s", app.GetName())
			log.Print(err)
			return
		}
		err = r.AddRoute(app.GetName(), c.IP())
		if err != nil {
			log.Printf("error on add route for %s with ip %s", app.GetName(), c.IP())
			log.Print(err)
		}
		u.Status = provision.StatusStarted
		err = p.collection().Update(bson.M{"name": u.Name}, u)
		if err != nil {
			log.Print(err)
		}
	}(p, app)
	return nil
}

func (p *LXCProvisioner) Restart(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/restart")
	if err != nil {
		msg := fmt.Sprintf("Failed to restart the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (p *LXCProvisioner) Deploy(a provision.App, w io.Writer) error {
	return deploy.Git(p, a, w)
}

func (p *LXCProvisioner) Destroy(app provision.App) error {
	c := container{name: app.GetName()}
	go func(c container) {
		log.Printf("stoping container %s", c.name)
		c.stop()
		log.Printf("destroying container %s", c.name)
		c.destroy()
		log.Printf("removing container %s from the database", c.name)
		p.collection().Remove(bson.M{"name": c.name})
	}(c)
	return nil
}

func (p *LXCProvisioner) Addr(app provision.App) (string, error) {
	r, err := p.router()
	if err != nil {
		return "", err
	}
	return r.Addr(app.GetName())
}

func (*LXCProvisioner) AddUnits(app provision.App, units uint) ([]provision.Unit, error) {
	return []provision.Unit{}, nil
}

func (*LXCProvisioner) RemoveUnit(app provision.App, unitName string) error {
	return nil
}

func (*LXCProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	return app.Run("/var/lib/tsuru/hooks/dependencies", w)
}

func (*LXCProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no"}
	arguments = append(arguments, app.ProvisionUnits()[0].GetIp())
	arguments = append(arguments, cmd)
	arguments = append(arguments, args...)
	return executor().Execute("ssh", arguments, nil, stdout, stderr)
}

func (p *LXCProvisioner) CollectStatus() ([]provision.Unit, error) {
	var units []provision.Unit
	err := p.collection().Find(nil).All(&units)
	if err != nil {
		return []provision.Unit{}, err
	}
	return units, nil
}

func (p *LXCProvisioner) collection() *mgo.Collection {
	name, err := config.GetString("lxc:collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Printf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
