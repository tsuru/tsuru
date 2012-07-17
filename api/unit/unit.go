package unit

import (
	"fmt"
	"github.com/timeredbull/tsuru/log"
	"os/exec"
	"strconv"
	"strings"
)

type Unit struct {
	Type          string
	Name          string
	Machine       int
	Ip            string
	AgentState    string `yaml:"agent-state"`
	InstanceState string `yaml:"instance-state"`
	InstanceId    string `yaml:"instance-id"`
}

func (u *Unit) Destroy() ([]byte, error) {
	cmd := exec.Command("juju", "destroy-service", u.Name)
	log.Printf("destroying %s with name %s", u.Type, u.Name)
	return cmd.CombinedOutput()
}

func (u *Unit) Command(cmds ...string) ([]byte, error) {
	c := exec.Command("juju", "ssh", "-o", "StrictHostKeyChecking no", strconv.Itoa(u.Machine))
	c.Args = append(c.Args, cmds...)
	log.Printf("executing %s on %s", strings.Join(cmds, " "), u.Name)
	return c.CombinedOutput()
}

func (u *Unit) ExecuteHook(hook string) ([]byte, error) {
	cmd := fmt.Sprintf("/var/lib/tsuru/hooks/%s", hook)
	output, err := u.Command(cmd)
	log.Print(string(output))
	return output, err
}
