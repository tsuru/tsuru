package local

import "os/exec"

// container represents an lxc container with the given name.
type container struct {
	name string
}

// create creates a lxc container with ubuntu template by default.
func (c *container) create() error {
	cmd := exec.Command("sudo", "lxc-create", "-t", "ubuntu", "-n", c.name)
	return cmd.Run()
}

// start starts a lxc container.
func (c *container) start() error {
	cmd := exec.Command("sudo", "lxc-start", "--daemon", "-n", c.name)
	return cmd.Run()
}
