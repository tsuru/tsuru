package cmd

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
)

func joinWithUserDir(p ...string) (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	paths := []string{user.HomeDir}
	paths = append(paths, p...)
	return path.Join(paths...), nil
}

func WriteToken(token string) error {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return err
	}
	file, err := os.Create(tokenPath)
	if err != nil {
		return err
	}
	_, err = file.WriteString(token)
	if err != nil {
		return err
	}
	return nil
}

func ReadToken() (string, error) {
	tokenPath, err := joinWithUserDir(".tsuru_token")
	if err != nil {
		return "", err
	}
	token, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}

type ServiceModel struct {
	Service   string
	Instances []string
}

func ShowServicesInstancesList(b []byte) ([]byte, error) {
	var services []cmd.ServiceModel
	err := json.Unmarshal(b, &services)
	if err != nil {
		return []byte{}, err
	}
	if len(services) == 0 {
		return []byte{}, nil
	}
	table := cmd.NewTable()
	table.Headers = cmd.Row([]string{"Services", "Instances"})
	for _, s := range services {
		insts := strings.Join(s.Instances, ", ")
		r := cmd.Row([]string{s.Service, insts})
		table.AddRow(r)
	}
	return table.Bytes(), nil
}
