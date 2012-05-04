package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"io/ioutil"
	"net/http"
)

type AppsCommand struct{}

func (c *AppsCommand) Run() error {
	response, err := http.Get("http://tsuru.plataformas.glb.com:4000/apps")
	if err != nil {
		return err
	}
	result, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	fmt.Println("app list")
	return c.Show(result)
}

func (c AppsCommand) Show(result []byte) error {
	var apps []app.App
	err := json.Unmarshal(result, &apps)
	if err != nil {
		fmt.Println(err, string(result))
		return err
	}
	for _, app := range apps {
		fmt.Println(app)
	}
	return nil
}

func (c *AppsCommand) Info() *Info {
	return &Info{Name: "apps"}
}
