package local

import (
	"github.com/globocom/tsuru/log"
	"os/exec"
)

// container represents an lxc container with the given name.
type container struct {
	name string
}

// create creates a lxc container with ubuntu template by default.
func (c *container) create() error {
	cmd := exec.Command("sudo", "lxc-create", "-t", "ubuntu", "-n", c.name)
	err := cmd.Run()
	if err != nil {
		log.Print(err)
	}
	return err
}

// start starts a lxc container.
func (c *container) start() error {
	cmd := exec.Command("sudo", "lxc-start", "--daemon", "-n", c.name)
	err := cmd.Run()
	if err != nil {
		log.Print(err)
	}
	return err
}

// stop stops a lxc container.
func (c *container) stop() error {
	cmd := exec.Command("sudo", "lxc-stop", "-n", c.name)
	err := cmd.Run()
	if err != nil {
		log.Print(err)
	}
	return err
}

// destroy destory a lxc container.
func (c *container) destroy() error {
	cmd := exec.Command("sudo", "lxc-destroy", "-n", c.name)
	err := cmd.Run()
	if err != nil {
		log.Print(err)
	}
	return err
}
