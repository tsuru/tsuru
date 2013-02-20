package local

import (
	"github.com/globocom/tsuru/fs"
	"github.com/globocom/tsuru/log"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"
)

var fsystem fs.Fs

func filesystem() fs.Fs {
	if fsystem == nil {
		fsystem = fs.OsFs{}
	}
	return fsystem
}

// container represents an lxc container with the given name.
type container struct {
	name string
}

// runCmd executes commands and log the given stdout and stderror.
func runCmd(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	log.Printf("running the cmd: %s with the args: %s", cmd, args)
	log.Print(string(out))
	return err
}

// ip returns the ip for the container.
func (c *container) ip() string {
	file, _ := filesystem().Open("/var/lib/misc/dnsmasq.leases")
	data, _ := ioutil.ReadAll(file)
	log.Print(string(data))
	time.Sleep(5 * time.Second)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Index(line, c.name) != -1 {
			log.Printf("ip in %s", line)
			return strings.Split(line, " ")[2]
		}
	}
	return ""
}

// create creates a lxc container with ubuntu template by default.
func (c *container) create() error {
	return runCmd("sudo", "lxc-create", "-t", "ubuntu", "-n", c.name)
}

// start starts a lxc container.
func (c *container) start() error {
	return runCmd("sudo", "lxc-start", "--daemon", "-n", c.name)
}

// stop stops a lxc container.
func (c *container) stop() error {
	return runCmd("sudo", "lxc-stop", "-n", c.name)
}

// destroy destory a lxc container.
func (c *container) destroy() error {
	return runCmd("sudo", "lxc-destroy", "-n", c.name)
}
