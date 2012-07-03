package service

import (
	"encoding/json"
	"errors"
	"github.com/timeredbull/tsuru/api/app"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	endpoint string
}

func (c *Client) issueRequest(path, method string, params map[string][]string) (*http.Response, error) {
	v := url.Values(params)
	var suffix string
	var body io.Reader
	if method == "DELETE" {
		suffix = "?" + v.Encode()
	} else {
		body = strings.NewReader(v.Encode())
	}
	url := strings.TrimRight(c.endpoint, "/") + "/" + strings.TrimLeft(path, "/") + suffix
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (c *Client) jsonFromResponse(resp *http.Response) (env map[string]string, err error) {
	defer resp.Body.Close()
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &env)
	return
}

func (c *Client) Create(instance *ServiceInstance) (envVars map[string]string, err error) {
	var resp *http.Response
	params := map[string][]string{
		"name":         []string{instance.Name},
		"service_host": []string{instance.Host},
	}
	if resp, err = c.issueRequest("/resources/", "POST", params); err == nil && resp.StatusCode < 300 {
		return c.jsonFromResponse(resp)
	} else {
		err = errors.New("Failed to create the instance: " + instance.Name)
	}
	return
}

func (c *Client) Destroy(instance *ServiceInstance) (err error) {
	var resp *http.Response
	params := map[string][]string{
		"service_host": []string{instance.Host},
	}
	if resp, err = c.issueRequest("/resources/"+instance.Name+"/", "DELETE", params); err == nil && resp.StatusCode > 299 {
		err = errors.New("Failed to destroy the instance: " + instance.Name)
	}
	return err
}

func (c *Client) Bind(instance *ServiceInstance, app *app.App) (envVars map[string]string, err error) {
	var resp *http.Response
	params := map[string][]string{
		"hostname":     []string{app.Units[0].Ip},
		"service_host": []string{instance.Host},
	}
	if resp, err = c.issueRequest("/resources/"+instance.Name+"/", "POST", params); err == nil && resp.StatusCode < 300 {
		return c.jsonFromResponse(resp)
	} else {
		err = errors.New("Failed to bind instance " + instance.Name + " to the app " + app.Name + ".")
	}
	return
}

func (c *Client) Unbind(instance *ServiceInstance, app *app.App) (err error) {
	var resp *http.Response
	params := map[string][]string{
		"service_host": []string{instance.Host},
	}
	url := "/resources/" + instance.Name + "/hostname/" + app.Name + "/"
	if resp, err = c.issueRequest(url, "DELETE", params); err == nil && resp.StatusCode > 299 {
		err = errors.New("Failed to unbind instance " + instance.Name + " from the app " + app.Name + ".")
	}
	return
}
