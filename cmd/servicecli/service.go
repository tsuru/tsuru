package main

import (
	"github.com/timeredbull/tsuru/cmd"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type Service struct{}

func (c *Service) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "service",
		Usage:   "service (init|list|create|remove|update) [args]",
		Desc:    "manage services.",
		MinArgs: 1,
	}
}

func (c *Service) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"create": &ServiceCreate{},
	}
}

type ServiceCreate struct{}

func (c *ServiceCreate) Info() *cmd.Info {
	desc := "Creates a service based on a passed manifest. The manifest format should be a yaml and follow the standard described in the documentation (should link to it here)"
	return &cmd.Info{
		Name:  "create",
		Usage: "service create path/to/manifesto",
		Desc:  desc,
	}
}

func (c *ServiceCreate) Run(context *cmd.Context, client cmd.Doer) error {
	manifest := context.Args[0]
	f, err := os.Open(manifest)
	if err != nil {
		return err
	}
	url := cmd.GetUrl("/service")
	var b []byte
	f.Read(b)
	body := strings.NewReader(string(b))
	request, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	b, err = ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, string(b))
	return nil
}
