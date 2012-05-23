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
	return &Info{
		Name:  "app",
		Usage: "app (create|remove|list|add-team|remove-team) [args]",
		Desc:  "manage your apps.",
		Args:  1,
	}
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
	return &Info{
		Name:  "add-team",
		Usage: "app add-team appname teamname",
		Desc:  "adds team to app.",
		Args:  2,
	}
}

func (c *AppAddTeam) Run(context *Context, client Doer) error {
	appName, teamName := context.Args[0], context.Args[1]
	url := GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
	request, err := http.NewRequest("PUT", url, nil)
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
	return &Info{
		Name:  "remove-team",
		Usage: "app remove-team appname teamname",
		Desc:  "removes team from app.",
		Args:  2,
	}
}

func (c *AppRemoveTeam) Run(context *Context, client Doer) error {
	appName, teamName := context.Args[0], context.Args[1]
	url := GetUrl(fmt.Sprintf("/apps/%s/%s", appName, teamName))
	request, err := http.NewRequest("DELETE", url, nil)
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
	request, err := http.NewRequest("GET", GetUrl("/apps"), nil)
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
	return &Info{
		Name:  "list",
		Usage: "app list",
		Desc:  "list your apps.",
	}
}

type AppCreate struct{}

func (c *AppCreate) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	b := bytes.NewBufferString(fmt.Sprintf(`{"name":"%s", "framework":"django"}`, appName))
	request, err := http.NewRequest("POST", GetUrl("/apps"), b)
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
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
	io.WriteString(context.Stdout, fmt.Sprintf(`App "%s" created with success!`+"\n", appName))
	io.WriteString(context.Stdout, fmt.Sprintf(`Your repository for "%s" project is "%s"`, appName, out["repository_url"])+"\n")
	return nil
}

func (c *AppCreate) Info() *Info {
	return &Info{
		Name:  "create",
		Usage: "app create appname",
		Desc:  "create a new app.",
		Args:  1,
	}
}

type AppRemove struct{}

func (c *AppRemove) Info() *Info {
	return &Info{
		Name:  "remove",
		Usage: "app remove appname",
		Desc:  "remove your app.",
		Args:  1,
	}
}

func (c *AppRemove) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	url := GetUrl(fmt.Sprintf("/apps/%s", appName))
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	io.WriteString(context.Stdout, fmt.Sprintf(`App "%s" removed with success!`+"\n", appName))
	return nil
}
