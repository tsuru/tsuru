package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"io"
	"io/ioutil"
	"net/http"
)

type AppsCommand struct{}

func (c *AppsCommand) Run(context *Context, client Doer) error {
	request, err := http.NewRequest("GET", "http://tsuru.plataformas.glb.com:8080/apps", nil)
	if err != nil {
		return err
	}
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

type CreateAppCommand struct{}

func (c *CreateAppCommand) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s", "framework":"django"}`, appName))
	request, err := http.NewRequest("POST", "http://tsuru.plataformas.glb.com:8080/apps", b)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf("Creating application: %s\n", appName))
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	out := make(map[string]string)
	err = json.Unmarshal(result, &out)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`Your repository for "%s" project is "%s"`, appName, out["repository_url"])+"\n")
	io.WriteString(context.Stdout, "Ok!")
	return nil
}

func (c *CreateAppCommand) Info() *Info {
	return &Info{Name: "create-app"}
}
