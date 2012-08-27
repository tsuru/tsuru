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
	conn ec2Connection
}

func (a *ec2Authorizer) connection() ec2Connection {
	if a.conn == nil {
		awsConfig, err := config.Get("aws")
		if err != nil {
			log.Panic(err)
		}
		m := awsConfig.(map[interface{}]interface{})
		region := aws.Region{EC2Endpoint: m["ec2-endpoint"].(string)}
		auth := aws.Auth{AccessKey: m["access-key"].(string), SecretKey: m["secret-key"].(string)}
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
