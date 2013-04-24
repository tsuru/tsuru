// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/router"
	_ "github.com/globocom/tsuru/router/nginx"
	_ "github.com/globocom/tsuru/router/testing"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

func init() {
	provision.Register("docker", &DockerProvisioner{})
}

var execut exec.Executor

func executor() exec.Executor {
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

func (p *DockerProvisioner) router() (router.Router, error) {
	r, err := config.GetString("router")
	if err != nil {
		return nil, err
	}
	return router.Get(r)
}

type DockerProvisioner struct{}

func (p *DockerProvisioner) setup(ip, framework string) error {
	formulasPath, err := config.GetString("docker:formulas-path")
	if err != nil {
		return err
	}
	log.Printf("Creating hooks dir for %s", ip)
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo mkdir -p /var/lib/tsuru/hooks"}
	err = executor().Execute("ssh", args, nil, nil, nil)
	if err != nil {
		log.Printf("error on creating hooks dir for %s", ip)
		log.Print(err)
		return err
	}
	log.Printf("Permissons on hooks dir for %s", ip)
	args = []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo chown -R ubuntu /var/lib/tsuru/hooks"}
	err = executor().Execute("ssh", args, nil, nil, nil)
	if err != nil {
		log.Printf("error on permissions for %s", ip)
		log.Print(err)
		return err
	}
	log.Printf("coping hooks to %s", ip)
	output := bytes.Buffer{}
	args = []string{"-q", "-o", "StrictHostKeyChecking no", "-r", formulasPath + "/" + framework + "/hooks", "ubuntu@" + ip + ":/var/lib/tsuru"}
	err = executor().Execute("scp", args, nil, &output, &output)
	if err != nil {
		log.Printf("error on execute scp with the args: %#v", args)
		log.Print(output.String())
		log.Print(err)
		return err
	}
	return nil
}

func (p *DockerProvisioner) install(ip string) error {
	log.Printf("executing the install hook for %s", ip)
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/install"}
	err := executor().Execute("ssh", args, nil, nil, nil)
	if err != nil {
		log.Printf("error on install for %s", ip)
		log.Print(err)
		return err
	}
	return nil
}

func (p *DockerProvisioner) start(ip string) error {
	args := []string{"-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/start"}
	err := executor().Execute("ssh", args, nil, nil, nil)
	if err != nil {
		log.Printf("error on start for %s", ip)
		log.Print(err)
		return err
	}
	return nil
}

// Provision creates a container and install its dependencies
//
// TODO (flaviamissi): make this atomic
func (p *DockerProvisioner) Provision(app provision.App) error {
	go func(p *DockerProvisioner, app provision.App) {
		c := container{name: app.GetName()}
		log.Printf("creating container %s", c.name)
		u := provision.Unit{
			Name:       app.GetName(),
			AppName:    app.GetName(),
			Type:       app.GetFramework(),
			Machine:    0,
			InstanceId: app.GetName(),
			Status:     provision.StatusCreating,
			Ip:         "",
		}
		log.Printf("inserting container unit %s in the database", app.GetName())
		if err := p.collection().Insert(u); err != nil {
			log.Print(err)
			return
		}
		instanceId, err := c.create()
		if err != nil {
			log.Printf("error on create container %s", app.GetName())
			log.Print(err)
			return
		}
		c.instanceId = instanceId
		u.InstanceId = instanceId
		if err := c.start(); err != nil {
			log.Printf("error on start container %s", app.GetName())
			log.Print(err)
			return
		}
		ip, err := c.ip() // handle this error
		u.Ip = ip
		u.Status = provision.StatusInstalling
		if err := p.collection().Update(bson.M{"name": u.Name}, u); err != nil {
			log.Print(err)
			return
		}
		if err := p.setup(ip, app.GetFramework()); err != nil {
			log.Printf("error on setup container %s", app.GetName())
			log.Print(err)
			return
		}
		if err := p.install(ip); err != nil {
			log.Printf("error on install container %s", app.GetName())
			log.Print(err)
			return
		}
		log.Printf("running provisioning start() for container %s", c.instanceId)
		if err := p.start(ip); err != nil {
			log.Printf("error on start app for container %s", app.GetName())
			log.Print(err)
			return
		}
		r, err := p.router()
		if err != nil {
			log.Print(err)
			return
		}
		err = r.AddRoute(app.GetName(), ip)
		if err != nil {
			log.Printf("error on add route for %s with ip %s", app.GetName(), ip)
			log.Print(err)
		}
		u.Status = provision.StatusStarted
		if err := p.collection().Update(bson.M{"name": u.Name}, u); err != nil {
			log.Print(err)
			return
		}
		log.Printf("Successfuly updated unit: %s", app.GetName())
	}(p, app)
	return nil
}

func (p *DockerProvisioner) Restart(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/restart")
	if err != nil {
		msg := fmt.Sprintf("Failed to restart the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (p *DockerProvisioner) Deploy(app provision.App, w io.Writer) error {
	return nil
}

func (p *DockerProvisioner) Destroy(app provision.App) error {
	units := app.ProvisionUnits()
	for _, u := range units {
		go func(u provision.AppUnit) {
			c := container{
				name: app.GetName(),
				// TODO: get actual c.instanceId
				instanceId: u.GetInstanceId(),
			}
			log.Printf("stoping container %s", u.GetInstanceId())
			if err := c.stop(); err != nil {
				log.Print("Could not stop container. Trying to destroy anyway.")
				log.Print(err.Error())
			}

			log.Printf("destroying container %s", u.GetInstanceId())
			if err := c.destroy(); err != nil {
				log.Print("Could not destroy container. Aborting...")
				log.Print(err.Error())
				return
			}

			log.Printf("removing container %s from the database", u.GetName())
			if err := p.collection().Remove(bson.M{"name": u.GetName()}); err != nil {
				log.Printf("Could not remove container from database. Error %s", err.Error())
			}
			log.Print("Units successfuly destroyed.")
		}(u)
	}
	return nil
}

func (*DockerProvisioner) Addr(app provision.App) (string, error) {
	units := app.ProvisionUnits()
	return units[0].GetIp(), nil
}

func (*DockerProvisioner) AddUnits(app provision.App, units uint) ([]provision.Unit, error) {
	return []provision.Unit{}, nil
}

func (*DockerProvisioner) RemoveUnit(app provision.App, unitName string) error {
	return nil
}

func (*DockerProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	return nil
}

func (*DockerProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no"}
	arguments = append(arguments, app.ProvisionUnits()[0].GetIp())
	arguments = append(arguments, cmd)
	arguments = append(arguments, args...)
	err := executor().Execute("ssh", arguments, nil, stdout, stderr)
	if err != nil {
		return err
	}
	return nil
}

func (p *DockerProvisioner) CollectStatus() ([]provision.Unit, error) {
	var units []provision.Unit
	err := p.collection().Find(nil).All(&units)
	if err != nil {
		return []provision.Unit{}, err
	}
	return units, nil
}

func (p *DockerProvisioner) collection() *mgo.Collection {
	name, err := config.GetString("docker:collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Printf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
