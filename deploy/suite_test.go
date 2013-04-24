package deploy

import (
	"github.com/globocom/config"
	"io"
	"launchpad.net/gocheck"
	"strings"
	"testing"
)

func Test(t *testing.T) { gocheck.TestingT(t) }

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) SetUpSuite(c *gocheck.C) {
	config.Set("git:unit-repo", "test/dir")
	config.Set("git:host", "tsuruhost.com")
}

func (s *S) TearDownSuite(c *gocheck.C) {
	config.Unset("git:unit-repo")
	config.Unset("git:host")
}

type fakeApp struct {
	name string
	cmds []string
}

func (a *fakeApp) Command(stdin io.Writer, stdout io.Writer, cmd ...string) error {
	a.cmds = append(a.cmds, strings.Join(cmd, " "))
	return nil
}

func (a *fakeApp) Restart(w io.Writer) error {
	a.cmds = append(a.cmds, "restart")
	return nil
}

func (a *fakeApp) InstallDeps(w io.Writer) error {
	a.cmds = append(a.cmds, "install deps")
	return nil
}

func (a *fakeApp) GetName() string {
	return a.name
}
