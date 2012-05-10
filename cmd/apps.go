package cmd

import (
	"encoding/json"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	"net/http"
)

type AppsCommand struct{}

func (c *AppsCommand) Run(context *Context, client Doer) error {
	request, err := http.NewRequest("GET", "http://tsuru.plataformas.glb.com:8080/apps", nil)
	if err != nil {
		return err
	}
	token, err := ReadToken()
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return c.Show([]byte(result), context)
}

func (c AppsCommand) Show(result []byte, context *Context) error {
	var apps []app.App
	err := json.Unmarshal(result, &apps)
	if err != nil {
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
