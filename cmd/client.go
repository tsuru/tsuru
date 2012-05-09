package cmd

import (
	"errors"
	"io/ioutil"
	"net/http"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Do(request *http.Request) ([]byte, error) {
	response, _ := http.DefaultClient.Do(request)
	defer response.Body.Close()
	result, _ := ioutil.ReadAll(response.Body)
	if response.StatusCode > 399 {
		return nil, errors.New(string(result))
	}
	return result, nil
}
