package ec2

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"os"
	"os/user"
	"path"
)

var EC2 *ec2.EC2
var pubKey []byte

func init() {
	Conn()
	getPubKey()
}

func getAuth() (*aws.Auth, error) {
	auth := new(aws.Auth)
	var err error
	auth.AccessKey, err = config.GetString("aws:access-key")
	if err != nil {
		msg := "Got error while reaading aws:access-key config options, have you set it?\nError is: " + err.Error()
		log.Print(msg)
		return nil, errors.New(msg)
	}
	auth.SecretKey, err = config.GetString("aws:secret-key")
	if err != nil {
		msg := "Got error while reaading aws:secret-key config options, have you set it?\nError is: " + err.Error()
		log.Print(msg)
		return nil, errors.New(msg)
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

func getPubKey() ([]byte, error) {
	u, err := user.Current()
	if err != nil {
		return []byte{}, err
	}
	files := []string{"id_dsa.pub", "id_rsa.pub", "identity.pub"}
	for i, f := range files {
		p := path.Join(u.HomeDir, ".ssh", f)
		pubKey, err = ioutil.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) && i != len(files)-1 {
				continue
			}
			return []byte{}, err
		}
		break
	}
	return pubKey, err
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
	cmd := fmt.Sprintf("\necho \"%s\" >> /root/.ssh/authorized_keys", pubKey)
	ud = append(ud, cmd...)
	rInst := &ec2.RunInstances{
		ImageId:  imageId,
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
