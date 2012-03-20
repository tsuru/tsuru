package unit

import "os/exec"

type Unit struct{
	Type string
}

func (u *Unit) Create(Name string) error {
	cmd := exec.Command("juju", "deploy", "--repository=/home/charms", "local:oneiric/" + u.Type, Name)
	return cmd.Start()
}
