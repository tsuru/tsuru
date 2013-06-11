// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/exec"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"github.com/globocom/tsuru/queue"
	"github.com/globocom/tsuru/router"
	_ "github.com/globocom/tsuru/router/hipache"
	_ "github.com/globocom/tsuru/router/nginx"
	_ "github.com/globocom/tsuru/router/testing"
	"io"
	"io/ioutil"
	"labix.org/v2/mgo"
	"net"
	"sync"
)

func init() {
	provision.Register("docker", &dockerProvisioner{})
}

var (
	execut exec.Executor
	emutex sync.Mutex
)

func executor() exec.Executor {
	emutex.Lock()
	defer emutex.Unlock()
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

func getRouter() (router.Router, error) {
	r, err := config.GetString("docker:router")
	if err != nil {
		return nil, err
	}
	return router.Get(r)
}

type dockerProvisioner struct{}

// Provision creates a route for the container
func (p *dockerProvisioner) Provision(app provision.App) error {
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to get router: %s", err.Error())
		return err
	}
	err = app.Ready()
	if err != nil {
		return err
	}
	return r.AddBackend(app.GetName())
}

func (p *dockerProvisioner) Restart(app provision.App) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		log.Printf("Got error while getting app containers: %s", err)
		return err
	}
	var buf bytes.Buffer
	for _, c := range containers {
		err = c.ssh(&buf, &buf, "/var/lib/tsuru/restart")
		if err != nil {
			log.Printf("Failed to restart %q: %s.", app.GetName(), err)
			log.Printf("Command outputs:")
			log.Printf("out: %s", buf)
			log.Printf("err: %s", buf)
			return err
		}
		buf.Reset()
	}
	return nil
}

func (p *dockerProvisioner) Deploy(a provision.App, version string, w io.Writer) error {
	_, err := deploy(a, version, w)
	if err != nil {
		return err
	}
	if containers, err := listAppContainers(a.GetName()); err == nil && len(containers) > 0 {
		for _, c := range containers {
			err = start(a, w)
			if err != nil {
				return err
			}
			if a.RemoveUnit(c.ID) != nil {
				c.remove()
			}
		}
	} else if err := start(a, w); err != nil {
		return err
	}
	a.Restart(w)
	app.Enqueue(queue.Message{
		Action: app.RegenerateApprcAndStart,
		Args:   []string{a.GetName()},
	})
	return nil
}

func (p *dockerProvisioner) Destroy(app provision.App) error {
	containers, _ := listAppContainers(app.GetName())
	for _, c := range containers {
		go func(c container) {
			c.remove()
		}(c)
	}
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to get router: %s", err.Error())
		return err
	}
	return r.RemoveBackend(app.GetName())
}

func (*dockerProvisioner) Addr(app provision.App) (string, error) {
	r, err := getRouter()
	if err != nil {
		log.Printf("Failed to get router: %s", err.Error())
		return "", err
	}
	addr, err := r.Addr(app.GetName())
	if err != nil {
		log.Printf("Failed to obtain app %s address: %s", app.GetName(), err.Error())
		return "", err
	}
	return addr, nil
}

func (*dockerProvisioner) AddUnits(a provision.App, units uint) ([]provision.Unit, error) {
	if units == 0 {
		return nil, errors.New("Cannot add 0 units")
	}
	containers, err := listAppContainers(a.GetName())
	if err != nil {
		return nil, err
	}
	if len(containers) < 1 {
		return nil, errors.New("New units can only be added after the first deployment")
	}
	version := containers[0].Version
	writer := app.LogWriter{App: a, Writer: ioutil.Discard}
	result := make([]provision.Unit, int(units))
	for i := uint(0); i < units; i++ {
		cmds, err := commandToRun(a)
		if err != nil {
			return nil, err
		}
		container, err := newContainer(a, cmds)
		if err != nil {
			return nil, err
		}
		go container.oldDeploy(version, &writer)
		result[i] = provision.Unit{
			Name:    container.ID,
			AppName: a.GetName(),
			Type:    a.GetPlatform(),
			Ip:      container.IP,
			Status:  provision.StatusInstalling,
		}
	}
	return result, nil
}

func (*dockerProvisioner) RemoveUnit(app provision.App, unitName string) error {
	container, err := getContainer(unitName)
	if err != nil {
		return err
	}
	if container.AppName != app.GetName() {
		return errors.New("Unit does not belong to this app")
	}
	return container.remove()
}

func (*dockerProvisioner) InstallDeps(app provision.App, w io.Writer) error {
	return nil
}

func (*dockerProvisioner) ExecuteCommand(stdout, stderr io.Writer, app provision.App, cmd string, args ...string) error {
	containers, err := listAppContainers(app.GetName())
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return errors.New("No containers for this app")
	}
	for _, c := range containers {
		err = c.ssh(stdout, stderr, cmd, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *dockerProvisioner) CollectStatus() ([]provision.Unit, error) {
	var containersGroup sync.WaitGroup
	var containers []container
	coll := collection()
	defer coll.Database.Session.Close()
	err := coll.Find(nil).All(&containers)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, nil
	}
	units := make(chan provision.Unit, len(containers))
	result := buildResult(len(containers), units)
	errs := make(chan error, len(containers))
	for _, container := range containers {
		containersGroup.Add(1)
		go collectUnit(container, units, errs, &containersGroup)
	}
	containersGroup.Wait()
	close(errs)
	close(units)
	if err, ok := <-errs; ok {
		return nil, err
	}
	return <-result, nil
}

func collectUnit(container container, units chan<- provision.Unit, errs chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	docker, _ := config.GetString("docker:binary")
	unit := provision.Unit{
		Name:    container.ID,
		AppName: container.AppName,
		Type:    container.Type,
	}
	switch container.Status {
	case "error":
		unit.Status = provision.StatusError
		units <- unit
		return
	case "created":
		return
	}
	out, err := runCmd(docker, "inspect", container.ID)
	if err != nil {
		errs <- err
		return
	}
	var c map[string]interface{}
	err = json.Unmarshal([]byte(out), &c)
	if err != nil {
		errs <- err
		return
	}
	unit.Ip = c["NetworkSettings"].(map[string]interface{})["IpAddress"].(string)
	if hostPort, err := container.hostPort(); err == nil && hostPort != container.HostPort {
		err = fixContainer(&container, unit.Ip, hostPort)
		if err != nil {
			errs <- err
			return
		}
	}
	addr := fmt.Sprintf("%s:%s", unit.Ip, container.Port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		unit.Status = provision.StatusInstalling
	} else {
		conn.Close()
		unit.Status = provision.StatusStarted
	}
	units <- unit
}

func buildResult(maxSize int, units <-chan provision.Unit) <-chan []provision.Unit {
	ch := make(chan []provision.Unit, 1)
	go func() {
		result := make([]provision.Unit, 0, maxSize)
		for unit := range units {
			result = append(result, unit)
		}
		ch <- result
	}()
	return ch
}

func fixContainer(container *container, ip, port string) error {
	router, err := getRouter()
	if err != nil {
		return err
	}
	router.RemoveRoute(container.AppName, container.getAddress())
	runCmd("ssh-keygen", "-R", container.IP)
	container.IP = ip
	container.HostPort = port
	router.AddRoute(container.AppName, container.getAddress())
	coll := collection()
	defer coll.Database.Session.Close()
	return coll.UpdateId(container.ID, container)
}

func (p *dockerProvisioner) SetCName(app provision.App, cname string) error {
	r, err := getRouter()
	if err != nil {
		return err
	}
	return r.SetCName(cname, app.GetName())
}

func (p *dockerProvisioner) UnsetCName(app provision.App, cname string) error {
	r, err := getRouter()
	if err != nil {
		return err
	}
	return r.UnsetCName(cname, app.GetName())
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
