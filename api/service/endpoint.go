package service

import (
	"encoding/json"
	"github.com/timeredbull/tsuru/api/bind"
	"github.com/timeredbull/tsuru/errors"
	"github.com/timeredbull/tsuru/log"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	endpoint string
}

func (c *Client) buildErrorMessage(err error, resp *http.Response) (msg string) {
	if err != nil {
		msg = err.Error()
	} else if resp != nil {
		b, _ := ioutil.ReadAll(resp.Body)
		msg = string(b)
	}
	return
}

func (c *Client) issueRequest(path, method string, params map[string][]string) (*http.Response, error) {
	log.Print("Issuing request...")
	v := url.Values(params)
	var suffix string
	var body io.Reader
	if method == "DELETE" || method == "GET" {
		suffix = "?" + v.Encode()
	} else {
		body = strings.NewReader(v.Encode())
	}
	url := strings.TrimRight(c.endpoint, "/") + "/" + strings.Trim(path, "/") + suffix
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		log.Print("Got error while creating request: " + err.Error())
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func (c *Client) jsonFromResponse(resp *http.Response) (env map[string]string, err error) {
	log.Print("Parsing response json...")
	defer resp.Body.Close()
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Print("Got error while parsing json: " + err.Error())
		return
	}
	err = json.Unmarshal(body, &env)
	return
}

func (c *Client) Create(instance *ServiceInstance) error {
	var err error
	log.Print("Attempting to call creation of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	var resp *http.Response
	params := map[string][]string{
		"name": []string{instance.Name},
	}
	if resp, err = c.issueRequest("/resources/", "POST", params); err == nil && resp.StatusCode < 300 {
		return nil
	} else {
		msg := "Failed to create the instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
		log.Print(msg)
		err = &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	return err
}

func (c *Client) Destroy(instance *ServiceInstance) (err error) {
	log.Print("Attempting to call destroy of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	var resp *http.Response
	if resp, err = c.issueRequest("/resources/"+instance.Name+"/", "DELETE", nil); err == nil && resp.StatusCode > 299 {
		msg := "Failed to destroy the instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
		log.Print(msg)
		err = &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	return err
}

func (c *Client) Bind(instance *ServiceInstance, app bind.App) (envVars map[string]string, err error) {
	log.Print("Attempting to call bind of service instance " + instance.Name + " and app " + app.GetName() + " at " + instance.ServiceName + " api")
	var resp *http.Response
	params := map[string][]string{
		"hostname": []string{app.GetUnits()[0].Ip},
	}
	if resp, err = c.issueRequest("/resources/"+instance.Name+"/", "POST", params); err == nil && resp.StatusCode < 300 {
		return c.jsonFromResponse(resp)
	} else if resp.StatusCode == http.StatusPreconditionFailed {
		err = &errors.Http{Code: resp.StatusCode, Message: "You cannot bind any app to this service instance because it is not ready yet."}
	} else {
		msg := "Failed to bind instance " + instance.Name + " to the app " + app.GetName() + ": " + c.buildErrorMessage(err, resp)
		log.Print(msg)
		err = &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	return
}

func (c *Client) Unbind(instance *ServiceInstance, app bind.App) (err error) {
	log.Print("Attempting to call unbind of service instance " + instance.Name + " and app " + app.GetName() + " at " + instance.ServiceName + " api")
	var resp *http.Response
	url := "/resources/" + instance.Name + "/hostname/" + app.GetUnits()[0].Ip + "/"
	if resp, err = c.issueRequest(url, "DELETE", nil); err == nil && resp.StatusCode > 299 {
		msg := "Failed to unbind instance " + instance.Name + " from the app " + app.GetName() + ": " + c.buildErrorMessage(err, resp)
		log.Print(msg)
		err = &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	return
}

// Connects into service's api
// The api should be prepared to receive the request,
// like below:
// GET /resources/<name>/status/
// The service host here is the private ip of the service instance
// 204 means the service is up, 500 means the service is down
func (c *Client) Status(instance *ServiceInstance) (string, error) {
	log.Print("Attempting to call status of service instance " + instance.Name + " at " + instance.ServiceName + " api")
	var (
		resp *http.Response
		err  error
	)
	url := "/resources/" + instance.Name + "/status/"
	if resp, err = c.issueRequest(url, "GET", nil); err == nil && resp.StatusCode == 204 {
		return "up", nil
	} else if err == nil && resp.StatusCode == 500 {
		return "down", nil
	} else {
		msg := "Failed to get status of instance " + instance.Name + ": " + c.buildErrorMessage(err, resp)
		log.Print(msg)
		err = &errors.Http{Code: http.StatusInternalServerError, Message: msg}
	}
	return "", err
}
