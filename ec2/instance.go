package ec2

import (
	"github.com/timeredbull/tsuru/config"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
)

var EC2 *ec2.EC2

func init() {
	Conn()
}

func getAuth() (*aws.Auth, error) {
	auth := new(aws.Auth)
	var err error
	auth.AccessKey, err = config.GetString("aws:access-key")
	if err != nil {
		return nil, err
	}
	auth.SecretKey, err = config.GetString("aws:secret-key")
	if err != nil {
		return nil, err
	}
	return auth, nil
}

func getRegion() (*aws.Region, error) {
	endpnt, err := config.GetString("aws:ec2-endpoint")
	if err != nil {
		return nil, err
	}
	region := &aws.Region{EC2Endpoint: endpnt}
	return region, nil
}

func Conn() (*ec2.EC2, error) {
	auth, err := getAuth()
	if err != nil {
		return nil, err
	}
	region, err := getRegion()
	if err != nil {
		return nil, err
	}
	EC2 = ec2.New(*auth, *region)
	return EC2, nil
}

func RunInstance(imageId string, userData string) (string, error) {
	ud := []byte(userData)
	rInst := &ec2.RunInstances{
		ImageId: imageId,
		UserData: ud,
		MinCount: 0,
		MaxCount: 0,
	}
	resp, err := EC2.RunInstances(rInst)
	if err != nil {
		return "", err
	}
	return resp.Instances[0].InstanceId, nil
}
