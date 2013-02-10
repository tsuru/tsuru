package local

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/provision"
	"io"
	"labix.org/v2/mgo"
	"os/exec"
)

type LocalProvisioner struct{}

func (*LocalProvisioner) Provision(app provision.App) error {
	container := container{name: app.GetName()}
	err := container.create()
	if err != nil {
		return err
	}
	err = container.start()
	if err != nil {
		return err
	}
	return nil
}

func (*LocalProvisioner) Destroy(app provision.App) error {
	container := container{name: app.GetName()}
	err := container.stop()
	if err != nil {
		return err
	}
	err = container.destroy()
	if err != nil {
		return err
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

func (*LocalProvisioner) CollectStatus() ([]provision.Unit, error) {
	return []provision.Unit{}, nil
}

func (p *LocalProvisioner) collection() *mgo.Collection {
	name, err := config.GetString("local:collection")
	if err != nil {
		log.Fatalf("FATAL: %s.", err)
	}
	conn, err := db.Conn()
	if err != nil {
		log.Printf("Failed to connect to the database: %s", err)
	}
	return conn.Collection(name)
}
