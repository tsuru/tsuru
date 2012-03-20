package unit

import "os/exec"

type Unit struct {
	Type string
	Name string
}

func (u *Unit) Create() error {
	cmd := exec.Command("juju", "deploy", "--repository=/home/charms", "local:oneiric/"+u.Type, u.Name)
	return cmd.Start()
}

func (u *Unit) Destroy() error {
	cmd := exec.Command("juju", "destroy-service", u.Name)
	return cmd.Start()
}
