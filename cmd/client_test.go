package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
)

type transport struct {
	msg    string
	status int
}

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp = &http.Response{
		Body:       ioutil.NopCloser(bytes.NewBufferString(t.msg)),
		StatusCode: t.status,
	}
	return resp, nil
}

func (s *S) TestShouldReturnBodyMessageOnError(c *C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)

	client := NewClient(&http.Client{Transport: &transport{msg: "You must be authenticated to execute this command.", status: http.StatusUnauthorized}})
	response, err := client.Do(request)
	c.Assert(response, IsNil)
	c.Assert(err.Error(), Equals, "You must be authenticated to execute this command.")
}

type conditionalTransport struct {
	transport
	condFunc func(*http.Request) bool
}

func (t *conditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.condFunc(req) {
		return &http.Response{Body: nil, StatusCode: 500}, errors.New("condition failed")
	}
	return t.transport.RoundTrip(req)
}

func (s *S) TestShouldReturnErrorWhenServerIsDown(c *C) {
	request, err := http.NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)
	client := NewClient(&http.Client{})
	_, err = client.Do(request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Server is down\n$")
}
