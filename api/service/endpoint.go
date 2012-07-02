package service

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	endpoint string
}

func (c *Client) issue(path, method string, params map[string][]string) (*http.Response, error) {
	v := url.Values(params)
	body := strings.NewReader(v.Encode())
	url := strings.TrimRight(c.endpoint, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (c *Client) Create(instance *ServiceInstance) (envVars map[string]string, err error) {
	var resp *http.Response
	params := map[string][]string{
		"name": []string{instance.Name},
	}
	resp, err = c.issue("/resources", "POST", params)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &envVars)
	return
}
