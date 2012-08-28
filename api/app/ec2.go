package app

import (
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
)

type ec2Connection interface {
	AuthorizeSecurityGroup(ec2.SecurityGroup, []ec2.IPPerm) (*ec2.SimpleResp, error)
	RevokeSecurityGroup(ec2.SecurityGroup, []ec2.IPPerm) (*ec2.SimpleResp, error)
}

type ec2Authorizer struct {
	conn           ec2Connection
	access, secret string
}

func (a *ec2Authorizer) connection() ec2Connection {
	if a.conn == nil {
		endpoint, err := config.GetString("aws:ec2-endpoint")
		if err != nil {
			log.Panic(err)
		}
		region := aws.Region{EC2Endpoint: endpoint}
		auth := aws.Auth{AccessKey: a.access, SecretKey: a.secret}
		a.conn = ec2.New(auth, region)
	}
	return a.conn
}

func (a *ec2Authorizer) authorize(app *App) error {
	group, perms := a.groupPerms(app)
	_, err := a.connection().AuthorizeSecurityGroup(group, perms)
	return err
}

func (a *ec2Authorizer) unauthorize(app *App) error {
	group, perms := a.groupPerms(app)
	_, err := a.connection().RevokeSecurityGroup(group, perms)
	return err
}

func (a *ec2Authorizer) setCreds(access, secret string) {
	a.access = access
	a.secret = secret
}

func (a *ec2Authorizer) groupPerms(app *App) (ec2.SecurityGroup, []ec2.IPPerm) {
	group := ec2.SecurityGroup{Name: "juju-" + app.JujuEnv}
	perms := []ec2.IPPerm{
		ec2.IPPerm{
			Protocol: "tcp",
			FromPort: 22,
			ToPort:   22,
		},
		ec2.IPPerm{
			Protocol: "tcp",
			FromPort: 80,
			ToPort:   80,
		},
	}
	return group, perms
}
