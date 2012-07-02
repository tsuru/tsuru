package service

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
)

func failHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}

type TestHandler struct {
	body   []byte
	method string
	url    string
}

func (h *TestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	content := `{"MYSQL_DATABASE_NAME": "CHICO", "MYSQL_HOST": "localhost", "MYSQL_PORT": "3306"}`
	h.method = r.Method
	h.url = r.URL.String()
	h.body, _ = ioutil.ReadAll(r.Body)
	w.Write([]byte(content))
}

func (s *S) TestCreateShouldSendTheNameOfTheResourceToTheEndpoint(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	_, err := client.Create(&instance)
	c.Assert(err, IsNil)
	expectedBody := "name=my-redis"
	c.Assert(err, IsNil)
	c.Assert(string(h.body), Equals, expectedBody)
	expectedUrl := "/resources"
	c.Assert(h.url, Equals, expectedUrl)
	c.Assert(h.method, Equals, "POST")
}

func (s *S) TestCreateShouldReturnTheMapWithTheEnvironmentVariables(c *C) {
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "your-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	env, err := client.Create(&instance)
	c.Assert(err, IsNil)
	c.Assert(env, DeepEquals, expected)
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURL(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/"+instance.Name)
	c.Assert(h.method, Equals, "DELETE")
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to destroy the instance: "+instance.Name+"$")
}
