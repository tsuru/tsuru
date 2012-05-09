package cmd

import (
	"errors"
	"io/ioutil"
	"net/http"
)

type Doer interface {
	Do(request *http.Request) ([]byte, error)
}

type Client struct {
	HttpClient *http.Client
}

func NewClient(client *http.Client) *Client {
	return &Client{HttpClient: client}
}

func (c *Client) Do(request *http.Request) ([]byte, error) {
	response, _ := c.HttpClient.Do(request)
	defer response.Body.Close()
	result, _ := ioutil.ReadAll(response.Body)
	if response.StatusCode > 399 {
		return nil, errors.New(string(result))
	}
	return result, nil
}
