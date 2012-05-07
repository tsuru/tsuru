package cmd

import (
	"encoding/json"
	"fmt"
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
		/* fmt.Println(err, string(result)) */
		context.Stderr.Write([]byte(err.Error()))
		return err
	}
	context.Stdout.Write([]byte("Application - State - Ip\n"))
	for _, app := range apps {
		context.Stdout.Write([]byte(fmt.Sprintf("%s - %s - %s\n", app.Name, app.State, app.Ip)))
		fmt.Println(app)
	}
	return nil
}

func (c *AppsCommand) Info() *Info {
	return &Info{Name: "apps"}
}
