package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/luizbafilho/fusis/api/types"
)

type Client struct {
	Addr       string
	HttpClient *http.Client
}

func NewClient(addr string) *Client {
	baseTimeout := 30 * time.Second
	fullTimeout := time.Minute
	return &Client{
		Addr: addr,
		HttpClient: &http.Client{
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   baseTimeout,
					KeepAlive: baseTimeout,
				}).Dial,
				TLSHandshakeTimeout: baseTimeout,
				// Disabled http keep alive for more reliable dial timeouts.
				MaxIdleConnsPerHost: -1,
				DisableKeepAlives:   true,
			},
			Timeout: fullTimeout,
		},
	}
}

func (c *Client) GetServices() ([]*types.Service, error) {
	resp, err := c.HttpClient.Get(c.path("services"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var services []*types.Service
	switch resp.StatusCode {
	case http.StatusOK:
		err = decode(resp.Body, &services)
	case http.StatusNoContent:
		services = []*types.Service{}
	default:
		return nil, formatError(resp)
	}
	return services, err
}

func (c *Client) GetService(id string) (*types.Service, error) {
	resp, err := c.HttpClient.Get(c.path("services", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var svc *types.Service
	switch resp.StatusCode {
	case http.StatusOK:
		err = decode(resp.Body, &svc)
	case http.StatusNotFound:
		return nil, types.ErrServiceNotFound
	default:
		return nil, formatError(resp)
	}
	return svc, err
}

func (c *Client) CreateService(svc types.Service) (string, error) {
	json, err := encode(svc)
	if err != nil {
		return "", err
	}
	resp, err := c.HttpClient.Post(c.path("services"), "application/json", json)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var id string
	switch resp.StatusCode {
	case http.StatusCreated:
		id = idFromLocation(resp)
	case http.StatusConflict:
		err = types.ErrServiceAlreadyExists
	default:
		err = formatError(resp)
	}
	return id, err
}

func (c *Client) DeleteService(id string) error {
	req, err := http.NewRequest("DELETE", c.path("services", id), nil)
	if err != nil {
		return err
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		err = types.ErrServiceNotFound
	case http.StatusNoContent:
	default:
		err = formatError(resp)
	}
	return err
}

func (c *Client) AddDestination(dst types.Destination) (string, error) {
	json, err := encode(dst)
	if err != nil {
		return "", err
	}
	resp, err := c.HttpClient.Post(c.path("services", dst.ServiceId, "destinations"), "application/json", json)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var id string
	switch resp.StatusCode {
	case http.StatusNotFound:
		err = types.ErrServiceNotFound
	case http.StatusConflict:
		err = types.ErrDestinationAlreadyExists
	case http.StatusCreated:
		id = idFromLocation(resp)
	default:
		err = formatError(resp)
	}
	return id, err
}

func (c *Client) DeleteDestination(serviceId, destinationId string) error {
	req, err := http.NewRequest("DELETE", c.path("services", serviceId, "destinations", destinationId), nil)
	if err != nil {
		return err
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		err = types.ErrDestinationNotFound
	case http.StatusNoContent:
	default:
		err = formatError(resp)
	}
	return err
}

func encode(obj interface{}) (io.Reader, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

func decode(body io.Reader, obj interface{}) error {
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, obj)
	if err != nil {
		return fmt.Errorf("unable to unmarshal body %q: %s", string(data), err)
	}
	return nil
}

func formatError(resp *http.Response) error {
	body, _ := ioutil.ReadAll(resp.Body)
	return fmt.Errorf("Request failed. Status Code: %v. Body: %q", resp.StatusCode, string(body))
}

func (c Client) path(paths ...string) string {
	return strings.Join(append([]string{strings.TrimRight(c.Addr, "/")}, paths...), "/")
}

func idFromLocation(resp *http.Response) string {
	parts := strings.Split(resp.Header.Get("Location"), "/")
	return parts[len(parts)-1]
}
