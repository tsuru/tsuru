package app

import (
	"errors"
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os/exec"
	"syscall"
)

type jujuEnv struct {
	AccessKey     string `yaml:"access-key"`
	SecretKey     string `yaml:"secret-key"`
	Ec2           string `yaml:"ec2-uri"`
	S3            string `yaml:"s3-uri"`
	JujuOrigin    string `yaml:"juju-origin"`
	Type          string `yaml:"type"`
	AdminSecret   string `yaml:"admin-secret"`
	ControlBucket string `yaml:"control-bucket"`
	Series        string `yaml:"default-series"`
	ImageId       string `yaml:"default-image-id"`
	InstanceType  string `yaml:"default-instance-type"`
}

func newJujuEnvConf(access, secret string) (jujuEnv, error) {
	ec2, err := config.GetString("juju:ec2")
	if err != nil {
		return jujuEnv{}, err
	}
	s3, err := config.GetString("juju:s3")
	if err != nil {
		return jujuEnv{}, err
	}
	jujuOrigin, err := config.GetString("juju:origin")
	if err != nil {
		return jujuEnv{}, err
	}
	series, err := config.GetString("juju:series")
	if err != nil {
		return jujuEnv{}, err
	}
	imageId, err := config.GetString("juju:image-id")
	if err != nil {
		return jujuEnv{}, err
	}
	instaceType, err := config.GetString("juju:instance-type")
	if err != nil {
		return jujuEnv{}, err
	}
	adminSecret, err := newUUID()
	if err != nil {
		return jujuEnv{}, err
	}
	controlBucket := fmt.Sprintf("juju-%s", adminSecret)
	return jujuEnv{
		Ec2:           ec2,
		S3:            s3,
		JujuOrigin:    jujuOrigin,
		Type:          "ec2",
		AdminSecret:   adminSecret,
		ControlBucket: controlBucket,
		Series:        series,
		ImageId:       imageId,
		InstanceType:  instaceType,
		AccessKey:     access,
		SecretKey:     secret,
	}, nil
}

func newEnvironConf(a *App) error {
	envs := map[string]map[string]jujuEnv{}
	file, err := filesystem().OpenFile(environConfPath, syscall.O_CREAT|syscall.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	err = goyaml.Unmarshal(content, &envs)
	if err != nil {
		return err
	}
	if _, ok := envs["environments"]; !ok {
		envs["environments"] = map[string]jujuEnv{}
	}
	jujuEnv, err := newJujuEnvConf(a.EC2Creds["access"], a.EC2Creds["secret"])
	if err != nil {
		return err
	}
	envs["environments"][a.Name] = jujuEnv
	data, err := goyaml.Marshal(&envs)
	ret, err := file.Seek(0, 0)
	if err != nil {
		return err
	}
	if ret != 0 {
		return fmt.Errorf("Unexpected error when creating juju environment for app %s.", a.Name)
	}
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// removes a environ from environment.yaml
func removeEnvironConf(a *App) error {
	file, err := filesystem().OpenFile(environConfPath, syscall.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	envs := map[string]map[string]jujuEnv{}
	err = goyaml.Unmarshal(content, &envs)
	delete(envs["environments"], a.Name)
	data, err := goyaml.Marshal(&envs)
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func bootstrap(a *App) error {
	if a.JujuEnv == "" {
		return jujuEnvEmptyError
	}
	cmd := exec.Command("juju", "bootstrap", "-e", a.JujuEnv)
	log.Printf("INFO: bootstraping juju environment %s for the app %s", a.JujuEnv, a.Name)
	log.Printf("DEBUG: executing command juju bootstrap -e %s", a.JujuEnv)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	log.Printf("DEBUG: command output: %s", outStr)
	if err != nil {
		msg := fmt.Sprintf("Failed to bootstrap juju env (%s): %s", err, outStr)
		log.Printf("ERROR: failed to bootstrap juju environment %s:\n%s", a.JujuEnv, outStr)
		log.Print("INFO: attempting to destroy keystone env due to error...")
		return errors.New(msg)
	}
	return nil
}

func newJujuEnviron(a *App) error {
	err := newEnvironConf(a)
	if err != nil {
		return err
	}
	// changes the object but doesn't save because this function
	// is supposed to be called from createApp, that is responsible
	// for saving the object.
	a.JujuEnv = a.Name
	return bootstrap(a)
}
