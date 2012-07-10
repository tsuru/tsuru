package ec2

import (
	"bytes"
	"fmt"
	cMocker "github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/log"
	. "launchpad.net/gocheck"
	stdlog "log"
    "strings"
)

func (s *S) TestRunInstanceShouldCallEucaToolsRunCommand(c *C) {
	out := "$*"
	p, err := cMocker.Add("euca-run-instances", out)
	c.Assert(err, IsNil)
	defer cMocker.Remove(p)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
    pubKey = []byte("i'm a public key, trust me")
    ud := `echo some user data script`
    expectedCmd := fmt.Sprintf(`.*euca-run-instances a-0a --user-data="echo %s >> /root/.ssh/authorized_keys%s.*"`, pubKey, ud)
	c.Assert(err, IsNil)
	_, err = RunInstance("a-0a", ud)
    fLog := strings.Replace(w.String(), "\n", "", -1)
    fLog = strings.Replace(fLog, `\n`, "", -1)
    c.Assert(fLog, Matches, expectedCmd)
}

func (s *S) TestParseEucaToolsReplyShouldReturnInstanceFilledWithInfo(c *C) {
	out := `RESERVATION     r-puei67hu      778bc1b2683540c5a61bb889a06e2022        default
INSTANCE        i-000000ea      ami-00000007                    pending         0               m1.small        2012-07-10T18:32:22.000Z        unknown zone    aki-00000002    ari-00000003
`
	p, err := cMocker.Add("euca-run-instances", out)
	c.Assert(err, IsNil)
	defer cMocker.Remove(p)
	inst, err := RunInstance("a-0a", `echo "some user data script"`)
	c.Assert(inst.Id, Equals, "i-000000ea")
	c.Assert(inst.State, Equals, "pending")
}
