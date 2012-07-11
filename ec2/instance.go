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
	"os/exec"
	"os/user"
	"path"
	"strings"
)

type Instance struct {
	Id    string
	State string
}

var EC2 *ec2.EC2
var pubKey []byte
var Auth *aws.Auth
var Region *aws.Region

func init() {
	getPubKey()
}

func loadData() {
	var err error
	Auth, err = getAuth()
	if err != nil {
		log.Print(err.Error())
	}
	log.Print("Found auth: " + Auth.SecretKey + "  " + Auth.AccessKey)
	Region, err = getRegion()
	if err != nil {
		log.Print(err.Error())
	}
	log.Print("Found region: " + Region.EC2Endpoint)
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
		msg := "Got error while reaading aws:ec2-endpoint config options, have you set it?\nError is: " + err.Error()
		log.Print(msg)
		return nil, errors.New(msg)
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
	loadData()
	EC2 = ec2.New(*Auth, *Region)
	return EC2, nil
}

// Run an instance using the ec2 api
func runInstance(imageId string, userData string) (string, error) {
	ud := []byte(userData)
	cmd := fmt.Sprintf("\necho \"%s\" >> /root/.ssh/authorized_keys", pubKey)
	ud = append(ud, cmd...)
	rInst := &ec2.RunInstances{
		ImageId:  imageId,
		UserData: ud,
		MinCount: 1,
		MaxCount: 1,
	}
	resp, err := EC2.RunInstances(rInst)
	if err != nil {
		return "", err
	}
	return resp.Instances[0].InstanceId, nil
}

// Run an instance using euca2ools command line
// We have both options because of a problem with goamz ec2 generated signature
// It is not working with nova, so we work arounding it with euca2ools
func RunInstance(imageId, userData string) (*Instance, error) {
	log.Print("Attempting to run instance with image " + imageId + "...")
	// should replace user data's new lines by a ;
	userData = fmt.Sprintf(`echo %s >> /root/.ssh/authorized_keys;%s`, pubKey, userData)
	cmd := exec.Command("euca-run-instances", imageId, "--user-data", userData)
	log.Print(fmt.Sprintf(`executing euca-run-instances %s --user-data="%s"`, imageId, userData))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	instance := parseOutput(out)
	return instance, nil
}

func parseOutput(out []byte) *Instance {
	sout := splitBySpace(string(out))
	inst := &Instance{}
	for i, v := range sout {
		if v == "INSTANCE" {
			inst.Id = sout[i+1]
			inst.State = sout[i+3]
			break
		}
	}
	return inst
}

// Filter euca2ools output by removing line breaks and spliting the result string in a slice
func splitBySpace(s string) []string {
	s = strings.Replace(s, "\n", " ", -1)
	str := strings.Split(s, " ")
	var filtered []string
	for _, v := range str {
		if !strings.Contains(v, " ") && v != "" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
