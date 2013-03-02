package local

import (
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
	provision.Register("local", &LocalProvisioner{})
}

type LocalProvisioner struct{}

func (p *LocalProvisioner) setup(ip, framework string) error {
	formulasPath, err := config.GetString("local:formulas-path")
	if err != nil {
		return err
	}
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo mkdir -p /var/lib/tsuru/hooks")
	err = cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "sudo chown -R ubuntu /var/lib/tsuru/hooks")
	err = cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("scp", "-q", "-o", "StrictHostKeyChecking no", formulasPath+"/"+framework+"/hooks/*", "ubuntu@"+ip+":/var/lib/tsuru/hooks")
	return cmd.Run()
}

func (p *LocalProvisioner) install(ip string) error {
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "/var/lib/tsuru/hooks/install")
	return cmd.Run()
}

func (p *LocalProvisioner) start(ip string) error {
	cmd := exec.Command("ssh", "-q", "-o", "StrictHostKeyChecking no", "-l", "ubuntu", ip, "/var/lib/tsuru/hooks/start")
	return cmd.Run()
}

func (p *LocalProvisioner) Provision(app provision.App) error {
	container := container{name: app.GetName()}
	log.Printf("creating container %s", app.GetName())
	err := container.create()
	if err != nil {
		log.Printf("error on create container %s", app.GetName())
		log.Print(err)
		return err
	}
	err = container.start()
	if err != nil {
		log.Printf("error on start container %s", app.GetName())
		log.Print(err)
		return err
	}
	ip := container.ip()
	err = p.setup(ip, app.GetFramework())
	if err != nil {
		log.Printf("error on setup container %s", app.GetName())
		log.Print(err)
	}
	err = p.install(ip)
	if err != nil {
		log.Printf("error on install container %s", app.GetName())
		log.Print(err)
	}
	err = p.start(ip)
	if err != nil {
		log.Printf("error on start app for container %s", app.GetName())
		log.Print(err)
	}
	u := provision.Unit{
		Name:       app.GetName(),
		AppName:    app.GetName(),
		Type:       app.GetFramework(),
		Machine:    0,
		InstanceId: app.GetName(),
		Status:     provision.StatusStarted,
		Ip:         ip,
	}
	log.Printf("inserting container unit %s in the database", app.GetName())
	return p.collection().Insert(u)
}

func (p *LocalProvisioner) Destroy(app provision.App) error {
	container := container{name: app.GetName()}
	log.Printf("destroying container %s", app.GetName())
	err := container.stop()
	if err != nil {
		log.Printf("error on stop container %s", app.GetName())
		log.Print(err)
		return err
	}
	err = container.destroy()
	if err != nil {
		log.Printf("error on destroy container %s", app.GetName())
		log.Print(err)
		return err
	}
	log.Printf("removing container %s from the database", app.GetName())
	return p.collection().Remove(bson.M{"name": app.GetName()})
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
