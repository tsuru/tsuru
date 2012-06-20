package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type App struct{}

func (c *App) Info() *Info {
	return &Info{
		Name:    "app",
		Usage:   "app (create|remove|list|add-team|remove-team) [args]",
		Desc:    "manage your apps.",
		MinArgs: 1,
	}
}

func (c *App) Subcommands() map[string]interface{} {
	return map[string]interface{}{
		"add-team":    &AppAddTeam{},
		"remove-team": &AppRemoveTeam{},
		"create":      &AppCreate{},
		"remove":      &AppRemove{},
		"list":        &AppList{},
		"run":         &AppRun{},
		"log":         &AppLog{},
	}
}

type AppAddTeam struct{}

func (c *AppAddTeam) Info() *Info {
	return &Info{
		Name:    "add-team",
		Usage:   "app add-team appname teamname",
		Desc:    "adds team to app.",
		MinArgs: 2,
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
		Name:    "remove-team",
		Usage:   "app remove-team appname teamname",
		Desc:    "removes team from app.",
		MinArgs: 2,
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

type AppModel struct {
	Name  string
	State string
	Ip    string
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
	var apps []AppModel
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
	io.WriteString(context.Stdout, fmt.Sprintf(`App "%s" successfully created!`+"\n", appName))
	io.WriteString(context.Stdout, fmt.Sprintf(`Your repository for "%s" project is "%s"`, appName, out["repository_url"])+"\n")
	return nil
}

func (c *AppCreate) Info() *Info {
	return &Info{
		Name:    "create",
		Usage:   "app create appname",
		Desc:    "create a new app.",
		MinArgs: 1,
	}
}

type AppRemove struct{}

func (c *AppRemove) Info() *Info {
	return &Info{
		Name:    "remove",
		Usage:   "app remove appname",
		Desc:    "remove your app.",
		MinArgs: 1,
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
	io.WriteString(context.Stdout, fmt.Sprintf(`App "%s" successfully removed!`+"\n", appName))
	return nil
}

type AppRun struct{}

func (c *AppRun) Info() *Info {
	desc := `run a command in all instances of the app, and prints the output.
Notice that you may need quotes to run your command if you want to deal with
input and outputs redirects, and pipes.
`
	return &Info{
		Name:    "run",
		Usage:   `app run appname command commandarg1 commandarg2 ... commandargn`,
		Desc:    desc,
		MinArgs: 1,
	}
}

func (c *AppRun) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	url := GetUrl(fmt.Sprintf("/apps/%s/run", appName))
	b := strings.NewReader(strings.Join(context.Args[1:], " "))
	request, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	r, err := client.Do(request)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.Copy(context.Stdout, r.Body)
	return err
}

type AppLog struct{}

func (c *AppLog) Info() *Info {
	return &Info{
		Name:    "log",
		Usage:   "app log appname",
		Desc:    "shows app log",
		MinArgs: 1,
	}
}

type Log struct {
	Date    time.Time
	Message string
}

func (c *AppLog) Run(context *Context, client Doer) error {
	appName := context.Args[0]
	url := GetUrl(fmt.Sprintf("/apps/%s/log", appName))
	request, err := http.NewRequest("GET", url, nil)
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
	logs := []Log{}
	err = json.Unmarshal(result, &logs)
	if err != nil {
		return err
	}
	for _, log := range logs {
		context.Stdout.Write([]byte(log.Date.String() + " - " + log.Message + "\n"))
	}
	return err
}
