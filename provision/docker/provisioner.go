// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"os/exec"
)

func init() {
	provision.Register("docker", &LocalProvisioner{})
}

type LocalProvisioner struct{}

func (p *LocalProvisioner) setup(ip, framework string) error {
	formulasPath, err := config.GetString("docker:formulas-path")
	if err != nil {
		return err
	}
	log.Printf("Creating hooks dir for %s", ip)
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo mkdir -p /var/lib/tsuru/hooks")
	err = cmd.Run()
	if err != nil {
		log.Printf("error on creating hooks dir for %s", ip)
		log.Print(err)
		return err
	}
	log.Printf("Permissons on hooks dir for %s", ip)
	cmd = exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo chown -R ubuntu /var/lib/tsuru/hooks")
	err = cmd.Run()
	if err != nil {
		log.Printf("error on permissions for %s", ip)
		log.Print(err)
		return err
	}
	log.Printf("coping hooks to %s", ip)
	cmd = exec.Command("scp", "-q", "-o", "StrictHostKeyChecking no", "-r", formulasPath+"/"+framework+"/hooks", "ubuntu@"+ip+":/var/lib/tsuru")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error on execute scp with the args: %#v", cmd.Args)
		log.Print(string(output))
		log.Print(err)
		return err
	}
	return nil
}

func (p *LocalProvisioner) install(ip string) error {
	log.Printf("executing the install hook for %s", ip)
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/install")
	err := cmd.Run()
	if err != nil {
		log.Printf("error on install for %s", ip)
		log.Print(err)
		return err
	}
	return nil
}

func (p *LocalProvisioner) start(ip string) error {
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo /var/lib/tsuru/hooks/start")
	err := cmd.Run()
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
func (p *LocalProvisioner) Provision(app provision.App) error {
	go func(p *LocalProvisioner, app provision.App) {
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
		instance_id, err := c.create()
		if err != nil {
			log.Printf("error on create container %s", app.GetName())
			log.Print(err)
			return
		}
		c.instanceId = instance_id
		u.InstanceId = instance_id
		if err = c.start(); err != nil {
			log.Printf("error on start container %s", app.GetName())
			log.Print(err)
			return
		}
		ip, err := c.ip()
		u.Ip = ip
		u.Status = provision.StatusInstalling
		if err = p.collection().Update(bson.M{"name": u.Name}, u); err != nil {
			log.Print(err)
			return
		}
		if err = p.setup(ip, app.GetFramework()); err != nil {
			log.Printf("error on setup container %s", app.GetName())
			log.Print(err)
			return
		}
		if err = p.install(ip); err != nil {
			log.Printf("error on install container %s", app.GetName())
			log.Print(err)
			return
		}
		log.Printf("running provisioning start() for container %s", c.instanceId)
		if err = p.start(ip); err != nil {
			log.Printf("error on start app for container %s", app.GetName())
			log.Print(err)
			return
		}
		if err = AddRoute(app.GetName(), ip); err != nil {
			log.Printf("error on add route for %s with ip %s", app.GetName(), ip)
			log.Print(err)
			return
		}
		if err = RestartRouter(); err != nil {
			log.Printf("error on restart router")
			log.Print(err)
			return
		}
		u.Status = provision.StatusStarted
		if err = p.collection().Update(bson.M{"name": u.Name}, u); err != nil {
			log.Print(err)
			return
		}
		log.Printf("Successfuly updated unit: %s", app.GetName())
	}(p, app)
	return nil
}

func (p *LocalProvisioner) Restart(app provision.App) error {
	var buf bytes.Buffer
	err := p.ExecuteCommand(&buf, &buf, app, "/var/lib/tsuru/hooks/restart")
	if err != nil {
		msg := fmt.Sprintf("Failed to restart the app (%s): %s", err, buf.String())
		app.Log(msg, "tsuru-provisioner")
		return &provision.Error{Reason: buf.String(), Err: err}
	}
	return nil
}

func (p *LocalProvisioner) Destroy(app provision.App) error {
	units := app.ProvisionUnits()
	for _, u := range units {
		go func(u provision.AppUnit) {
			c := container{
				name: app.GetName(),
				// TODO: get actual c.instanceId
				instanceId: u.GetInstanceId(),
			}
			log.Printf("stoping container %s", u.GetInstanceId())
			c.stop()

			log.Printf("destroying container %s", u.GetInstanceId())
			c.destroy()

			log.Printf("removing container %s from the database", u.GetName())
			p.collection().Remove(bson.M{"name": u.GetName()})
		}(u)
	}
	return nil
}

func (*LocalProvisioner) Addr(app provision.App) (string, error) {
	units := app.ProvisionUnits()
	return units[0].GetIp(), nil
}

func (*LocalProvisioner) AddUnits(app provision.App, units uint) ([]provision.Unit, error) {
	return []provision.Unit{}, nil
}

func (*LocalProvisioner) RemoveUnit(app provision.App, unitName string) error {
	return nil
}

func (*LocalProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	arguments := []string{"-l", "ubuntu", "-q", "-o", "StrictHostKeyChecking no"}
	arguments = append(arguments, app.ProvisionUnits()[0].GetIp())
	arguments = append(arguments, cmd)
	arguments = append(arguments, args...)
	c := exec.Command("ssh", arguments...)
	c.Stdout = stdout
	c.Stderr = stderr
	err := c.Run()
	if err != nil {
		return err
	}
	return nil
}

func (p *LocalProvisioner) CollectStatus() ([]provision.Unit, error) {
	var units []provision.Unit
	err := p.collection().Find(nil).All(&units)
	if err != nil {
		return []provision.Unit{}, err
	}
	return units, nil
}

func (p *LocalProvisioner) collection() *mgo.Collection {
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
