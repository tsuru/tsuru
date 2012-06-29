package ec2

import (
	"github.com/timeredbull/tsuru/config"
	"launchpad.net/goamz/aws"
)

func getAuth() (*aws.Auth, error) {
	auth := new(aws.Auth)
	var err error
	auth.AccessKey, err = config.GetString("ec2:access-key")
	if err != nil {
		return nil, err
	}
	auth.SecretKey, err = config.GetString("ec2:secret-key")
	if err != nil {
		return nil, err
	}
	return auth, nil
}
