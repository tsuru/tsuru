package cmd

import (
	"encoding/json"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	"net/http"
)

type AppsCommand struct{}

func (c *AppsCommand) Run(context *Context) error {
	response, err := http.Get("http://tsuru.plataformas.glb.com:4000/apps")
	if err != nil {
		return err
	}
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return c.Show(result, context)
}

func (c AppsCommand) Show(result []byte, context *Context) error {
	var apps []app.App
	err := json.Unmarshal(result, &apps)
	if err != nil {
		context.Stderr.Write([]byte(err.Error()))
		return err
	}
	table := NewTable()
	table.Headers = Row{"Application", "State", "Ip"}
	for _, app := range apps {
		table.AddRow(Row{app.Name, app.State, app.Ip})
	}
	context.Stdout.Write(table.Bytes())
	return nil
}

func (c *AppsCommand) Info() *Info {
	return &Info{Name: "apps"}
}
