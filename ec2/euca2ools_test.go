package ec2

import (
	"bytes"
	"fmt"
	cMocker "github.com/timeredbull/commandmocker"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	. "launchpad.net/gocheck"
	stdlog "log"
	"os"
	"strings"
)

func (s *S) TestShouldExportEc2EnvVarsFromTsuruConf(c *C) {
	oldsKey, err := config.GetString("aws:secret-key")
	c.Assert(err, IsNil)
	oldaKey, err := config.GetString("aws:access-key")
	c.Assert(err, IsNil)
	oldEndpnt, err := config.GetString("aws:ec2-endpoint")
	c.Assert(err, IsNil)
	defer func() {
		config.Set("aws:secret-key", oldsKey)
		config.Set("aws:access-key", oldaKey)
		config.Set("aws:ec2-endpoint", oldEndpnt)
	}()
	config.Set("aws:secret-key", "super-ultra-power-secret")
	config.Set("aws:access-key", "not-that-secret")
	config.Set("aws:ec2-endpoint", "http://some-cool-endpoint.com/services/Cloud")
	err = configureEc2Env()
	c.Assert(err, IsNil)
	sKey := os.Getenv("EC2_SECRET_KEY")
	aKey := os.Getenv("EC2_ACCESS_KEY")
	endpnt := os.Getenv("EC2_URL")
	c.Assert(sKey, Equals, "super-ultra-power-secret")
	c.Assert(aKey, Equals, "not-that-secret")
	c.Assert(endpnt, Equals, "http://some-cool-endpoint.com/services/Cloud")
}

func (s *S) TestrunInstanceShouldCallEucaToolsRunCommand(c *C) {
	out := "$*"
	p, err := cMocker.Add("euca-run-instances", out)
	c.Assert(err, IsNil)
	defer cMocker.Remove(p)
	w := bytes.NewBuffer([]byte{})
	l := stdlog.New(w, "", stdlog.LstdFlags)
	log.Target = l
	pubKey = []byte("i'm a public key, trust me")
	ud := `echo some user data script`
	expectedCmd := fmt.Sprintf(`.*euca-run-instances a-0a --user-data="echo %s >> /root/.ssh/authorized_keys;%s.*"`, pubKey, ud)
	c.Assert(err, IsNil)
	_, err = runInstance("a-0a", ud)
	fLog := strings.Replace(w.String(), "\n", "", -1)
	fLog = strings.Replace(fLog, `\n`, "", -1)
	c.Assert(fLog, Matches, expectedCmd)
}

func (s *S) TestParseEucaToolsReplyShouldReturnInstanceFilledWithIdAndState(c *C) {
	out := `RESERVATION     r-k5v41ths      778bc1b2683540c5a61bb889a06e2022        default
INSTANCE        i-000000f4      ami-00000009                    pending         0               m1.small        2012-07-11T20:59:17.000Z        unknown zone    aki-00000002    ari-00000003
`
	p, err := cMocker.Add("euca-run-instances", out)
	c.Assert(err, IsNil)
	defer cMocker.Remove(p)
	inst, err := runInstance("a-0a", `echo "some user data script"`)
	c.Assert(inst.Id, Equals, "i-000000f4")
	c.Assert(inst.State, Equals, "pending")
}

func (s *S) TestSplitBySpace(c *C) {
	str := "FOOO      BAR  FOOBAR"
	slc := splitBySpace(str)
	c.Assert(slc, DeepEquals, []string{"FOOO", "BAR", "FOOBAR"})
}

func (s *S) TestSplitBySpaceWithLineBreak(c *C) {
	str := "FOOO      BAR  FOOBAR \n BARFOO"
	slc := splitBySpace(str)
	c.Assert(slc, DeepEquals, []string{"FOOO", "BAR", "FOOBAR", "BARFOO"})
}
