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

type App struct{}

func (c *App) Info() *Info {
	return &Info{Name: "app"}
}

func (c *App) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"add-team":    &AppAddTeam{},
		"remove-team": &AppRemoveTeam{},
		"create":      &AppCreate{},
		"remove":      &AppRemove{},
		"list":        &AppList{},
	}
}

type AppAddTeam struct{}

func (c *AppAddTeam) Info() *Info {
	return &Info{Name: "add-team"}
}

func (c *AppAddTeam) Run(context *Context, client Doer) error {
	appName, teamName := context.Args[0], context.Args[1]
	request, err := http.NewRequest("PUT", fmt.Sprintf("http://tsuru.plataformas.glb.com:8080/apps/%s/%s", appName, teamName), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`Team "%s" was added to the "%s" app`+"\n", teamName, appName))
	return nil
}

type AppRemoveTeam struct{}

func (c *AppRemoveTeam) Info() *Info {
	return &Info{Name: "remove-team"}
}

func (c *AppRemoveTeam) Run(context *Context, client Doer) error {
	appName, teamName := context.Args[0], context.Args[1]
	request, err := http.NewRequest("DELETE", fmt.Sprintf("http://tsuru.plataformas.glb.com:8080/apps/%s/%s", appName, teamName), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`Team "%s" was removed from the "%s" app`+"\n", teamName, appName))
	return nil
}

type AppList struct{}

func (c *AppList) Run(context *Context, client Doer) error {
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

func (c *AppList) Show(result []byte, context *Context) error {
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

func (c *AppList) Info() *Info {
	return &Info{Name: "list"}
}

type AppCreate struct{}

func (c *AppCreate) Run(context *Context, client Doer) error {
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

func (c *AppCreate) Info() *Info {
	return &Info{Name: "create-app"}
}

type AppRemove struct{}

func (c *AppRemove) Info() *Info {
	return &Info{Name: "delete-app"}
}

func (c *AppRemove) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	request, err := http.NewRequest("DELETE", fmt.Sprintf("http://tsuru.plataformas.glb.com:8080/apps/%s", appName), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf("App %s delete with success!", appName))
	return nil
}
