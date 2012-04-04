package unit

import (
	"fmt"
	"log"
	"os/exec"
)

type Unit struct {
	Type string
	Name string
}

func (u *Unit) Create() error {
	cmd := exec.Command("juju", "deploy", "--repository=/home/charms", "local:oneiric/"+u.Type, u.Name)
	log.Printf("deploying %s with name %s", u.Type, u.Name)
	return cmd.Start()
}

func (u *Unit) Destroy() error {
	cmd := exec.Command("juju", "destroy-service", u.Name)
	log.Printf("destroying %s with name %s", u.Type, u.Name)
	return cmd.Start()
}

func (u *Unit) AddRelation(su *Unit) error {
	cmd := exec.Command("juju", "add-relation", u.Name, su.Name)
	log.Printf("relating %s with service %s", u.Name, su.Name)
	return cmd.Start()
}

func (u *Unit) RemoveRelation(su *Unit) error {
	cmd := exec.Command("juju", "remove-relation", u.Name, su.Name)
	log.Printf("unrelating %s with service %s", u.Name, su.Name)
	return cmd.Start()
}

func (u *Unit) Command(command string) ([]byte, error) {
	cmd := exec.Command("juju", "ssh", "-o", "StrictHostKeyChecking no", u.Name+"/0", command)
	log.Printf("executing %s on %s", command, u.Name)
	return cmd.CombinedOutput()
}

func (u *Unit) SendFile(srcPath, dstPath string) error {
	cmd := exec.Command("juju", "scp", "-r", "-o", "StrictHostKeyChecking no", srcPath, u.Name+"/0:"+dstPath)
	log.Printf("sending %s to %s on %s", srcPath, dstPath, u.Name)
	return cmd.Start()
}

func (u *Unit) ExecuteHook(hook string) error {
	cmd := fmt.Sprintf("/var/lib/tsuru/hooks/%s", hook)
	output, err := u.Command(cmd)
	log.Printf(string(output))
	return err
}
