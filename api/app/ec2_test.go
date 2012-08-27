package app

import (
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
)

type Ec2Suite struct{}

var _ = Suite(&Ec2Suite{})

func (s *Ec2Suite) SetUpSuite(c *C) {
	err := config.ReadConfigFile("../../etc/tsuru.conf")
	c.Assert(err, IsNil)
}

func (s *Ec2Suite) TestEc2AuthorizerIsAnAuthorizer(c *C) {
	var a authorizer
	a = &ec2Authorizer{} // does not compile if the type does not implement the interface
	_ = a
}

func (s *Ec2Suite) TestEc2AuthorizerConnectionNonNil(c *C) {
	conn := &fakeEc2Conn{}
	authorizer := &ec2Authorizer{}
	authorizer.conn = conn
	got := authorizer.connection()
	c.Assert(got, DeepEquals, conn)
}

func (s *Ec2Suite) TestEc2AuthorizerConnectionNil(c *C) {
	srv, err := ec2test.NewServer()
	c.Assert(err, IsNil)
	defer srv.Quit()
	old, err := config.GetString("aws:ec2-endpoint")
	c.Assert(err, IsNil)
	config.Set("aws:ec2-endpoint", srv.URL())
	defer config.Set("aws:endpoint", old)
	authorizer := &ec2Authorizer{}
	authorizer.conn = nil
	got := authorizer.connection()
	c.Assert(got, FitsTypeOf, &ec2.EC2{})
	c.Assert(got.(*ec2.EC2).EC2Endpoint, Equals, srv.URL())
}

func (s *Ec2Suite) TestEc2AuthorizerAuthorize(c *C) {
	app := App{Name: "military_wives", JujuEnv: "military"}
	conn := fakeEc2Conn{}
	authorizer := &ec2Authorizer{conn: &conn}
	err := authorizer.authorize(&app)
	c.Assert(err, IsNil)
	actionSsh := "authorize group juju-military. Protocol: tcp\nFromPort: 22\nToPort: 22"
	actionHttp := "authorize group juju-military. Protocol: tcp\nFromPort: 80\nToPort: 80"
	c.Assert(conn.hasAction(actionSsh), Equals, true)
	c.Assert(conn.hasAction(actionHttp), Equals, true)
}

func (s *Ec2Suite) TestEc2AuthorizerUnauthorize(c *C) {
	app := App{Name: "military_wives", JujuEnv: "military"}
	conn := fakeEc2Conn{}
	authorizer := &ec2Authorizer{conn: &conn}
	err := authorizer.unauthorize(&app)
	c.Assert(err, IsNil)
	actionSsh := "revoke group juju-military. Protocol: tcp\nFromPort: 22\nToPort: 22"
	actionHttp := "revoke group juju-military. Protocol: tcp\nFromPort: 80\nToPort: 80"
	c.Assert(conn.hasAction(actionSsh), Equals, true)
	c.Assert(conn.hasAction(actionHttp), Equals, true)
}

type fakeEc2Conn struct {
	actions []string
}

func (f *fakeEc2Conn) hasAction(action string) bool {
	for _, a := range f.actions {
		if a == action {
			return true
		}
	}
	return false
}

func (f *fakeEc2Conn) AuthorizeSecurityGroup(group ec2.SecurityGroup, perms []ec2.IPPerm) (*ec2.SimpleResp, error) {
	for _, perm := range perms {
		action := fmt.Sprintf("authorize group %s. Protocol: %s\nFromPort: %d\nToPort: %d", group.Name, perm.Protocol, perm.FromPort, perm.ToPort)
		f.actions = append(f.actions, action)
	}
	return &ec2.SimpleResp{}, nil
}

func (f *fakeEc2Conn) RevokeSecurityGroup(group ec2.SecurityGroup, perms []ec2.IPPerm) (*ec2.SimpleResp, error) {
	for _, perm := range perms {
		action := fmt.Sprintf("revoke group %s. Protocol: %s\nFromPort: %d\nToPort: %d", group.Name, perm.Protocol, perm.FromPort, perm.ToPort)
		f.actions = append(f.actions, action)
	}
	return &ec2.SimpleResp{}, nil
}

func createGroup(group, endpoint string) error {
	region := aws.Region{EC2Endpoint: endpoint}
	auth := aws.Auth{AccessKey: "access", SecretKey: "secret"}
	ec2Conn := ec2.New(auth, region)
	_, err := ec2Conn.CreateSecurityGroup(group, "anything")
	return err
}
