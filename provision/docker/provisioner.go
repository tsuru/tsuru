// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
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

func getRouter() (router.Router, error) {
	r, err := config.GetString("router")
	if err != nil {
		return nil, err
	}
	return router.Get(r)
}

type DockerProvisioner struct{}

// Provision creates a container and install its dependencies
func (p *DockerProvisioner) Provision(app provision.App) error {
	return nil
}

func (p *DockerProvisioner) Restart(app provision.App) error {
	return nil
}

func (p *DockerProvisioner) Deploy(app provision.App, w io.Writer) error {
	c, err := newContainer(app, deployContainerCmd)
	if err != nil {
		return err
	}
	img := image{Name: app.GetName()}
	if _, err := img.commit(c.id); err != nil {
		return err
	}
	if err := c.remove(); err != nil {
		return err
	}
	_, err = newContainer(app, runContainerCmd)
	return err
}

func (p *DockerProvisioner) Destroy(app provision.App) error {
	units := app.ProvisionUnits()
	for _, u := range units {
		go func(u provision.AppUnit) {
			c := container{
				name: app.GetName(),
				// TODO: get actual c.id
				id: u.GetInstanceId(),
			}
			log.Printf("stoping container %s", u.GetInstanceId())
			if err := c.stop(); err != nil {
				log.Print("Could not stop container. Trying to remove it anyway.")
				log.Print(err.Error())
			}

			log.Printf("removing container %s", u.GetInstanceId())
			if err := c.remove(); err != nil {
				log.Print("Could not remove container. Aborting...")
				log.Print(err.Error())
				return
			}

			log.Printf("removing container %s from the database", u.GetName())
			if err := collection().Remove(bson.M{"name": u.GetName()}); err != nil {
				log.Printf("Could not remove container from database. Error %s", err.Error())
			}
			log.Print("Units successfuly removed.")
		}(u)
	}
	img := &image{Name: app.GetName()}
	log.Printf("removing image %s from the database", app.GetName())
	if err := img.remove(); err != nil {
		return err
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
	return nil
}

func (p *DockerProvisioner) CollectStatus() ([]provision.Unit, error) {
	var units []provision.Unit
	err := collection().Find(nil).All(&units)
	if err != nil {
		return []provision.Unit{}, err
	}
	return units, nil
}

func collection() *mgo.Collection {
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

func imagesCollection() *mgo.Collection {
	nameIndex := mgo.Index{Key: []string{"name"}, Unique: true}
	conn, err := db.Conn()
	if err != nil {
		log.Printf("Failed to connect to the database: %s", err)
	}
	c := conn.Collection("docker_image")
	c.EnsureIndex(nameIndex)
	return c
}
