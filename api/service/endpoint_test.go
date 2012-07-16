package service

import (
	"errors"
	"github.com/timeredbull/tsuru/api/unit"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

func failHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Server failed to do its job."))
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
	instance := ServiceInstance{Name: "my-redis", ServiceName: "redis", Host: "127.0.0.1"}
	client := &Client{endpoint: ts.URL}
	_, err := client.Create(&instance)
	c.Assert(err, IsNil)
	expectedUrl := "/resources/"
	c.Assert(h.url, Equals, expectedUrl)
	c.Assert(h.method, Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, IsNil)
	c.Assert(map[string][]string(v), DeepEquals, map[string][]string{"name": []string{"my-redis"}, "service_host": []string{"127.0.0.1"}})
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
	instance := ServiceInstance{Name: "your-redis", ServiceName: "redis", Host: "127.0.0.1"}
	client := &Client{endpoint: ts.URL}
	env, err := client.Create(&instance)
	c.Assert(err, IsNil)
	c.Assert(env, DeepEquals, expected)
}

func (s *S) TestCreateShouldReturnErrorIfTheRequestFail(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis", Host: "127.0.0.1"}
	client := &Client{endpoint: ts.URL}
	_, err := client.Create(&instance)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to create the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestDestroyShouldSendADELETERequestToTheResourceURLWithGetParameters(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis", Host: "127.0.0.1"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/"+instance.Name+"/?service_host=127.0.0.1")
	c.Assert(h.method, Equals, "DELETE")
}

func (s *S) TestDestroyShouldReturnErrorIfTheRequestFails(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "his-redis", ServiceName: "redis", Host: "127.0.0.1"}
	client := &Client{endpoint: ts.URL}
	err := client.Destroy(&instance)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to destroy the instance "+instance.Name+": Server failed to do its job.$")
}

func (s *S) TestBindShouldSendAPOSTToTheResourceURL(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis", Host: "127.0.0.1"}
	a := App{
		Name: "her-app",
		Units: []unit.Unit{
			unit.Unit{
				Ip: "10.0.10.1",
			},
		},
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, &a)
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/"+instance.Name+"/")
	c.Assert(h.method, Equals, "POST")
	v, err := url.ParseQuery(string(h.body))
	c.Assert(err, IsNil)
	c.Assert(map[string][]string(v), DeepEquals, map[string][]string{"hostname": []string{"10.0.10.1"}, "service_host": []string{"127.0.0.1"}})
}

func (s *S) TestBindShouldReturnMapWithTheEnvironmentVariable(c *C) {
	expected := map[string]string{
		"MYSQL_DATABASE_NAME": "CHICO",
		"MYSQL_HOST":          "localhost",
		"MYSQL_PORT":          "3306",
	}
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis", Host: "127.0.0.1"}
	a := App{
		Name: "her-app",
		Units: []unit.Unit{
			unit.Unit{
				Ip: "10.0.10.1",
			},
		},
	}
	client := &Client{endpoint: ts.URL}
	env, err := client.Bind(&instance, &a)
	c.Assert(err, IsNil)
	c.Assert(env, DeepEquals, expected)
}

func (s *S) TestBindShouldreturnErrorIfTheRequestFail(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "her-redis", ServiceName: "redis", Host: "127.0.0.1"}
	a := App{
		Name: "her-app",
		Units: []unit.Unit{
			unit.Unit{
				Ip: "10.0.10.1",
			},
		},
	}
	client := &Client{endpoint: ts.URL}
	_, err := client.Bind(&instance, &a)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to bind instance her-redis to the app her-app: Server failed to do its job.$")
}

func (s *S) TestUnbindSendADELETERequestToTheResourceURL(c *C) {
	h := TestHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven", Host: "192.168.1.10"}
	a := App{
		Name: "arch-enemy",
		Units: []unit.Unit{
			unit.Unit{
				Ip: "2.2.2.2",
			},
		},
	}
	client := &Client{endpoint: ts.URL}
	err := client.Unbind(&instance, &a)
	c.Assert(err, IsNil)
	c.Assert(h.url, Equals, "/resources/heaven-can-wait/hostname/2.2.2.2/?service_host=192.168.1.10")
	c.Assert(h.method, Equals, "DELETE")
}

func (s *S) TestUnbindReturnsErrorIfTheRequestFails(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(failHandler))
	defer ts.Close()
	instance := ServiceInstance{Name: "heaven-can-wait", ServiceName: "heaven", Host: "192.168.1.10"}
	a := App{
		Name: "arch-enemy",
		Units: []unit.Unit{
			unit.Unit{
				Ip: "2.2.2.2",
			},
		},
	}
	client := &Client{endpoint: ts.URL}
	err := client.Unbind(&instance, &a)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to unbind instance heaven-can-wait from the app arch-enemy: Server failed to do its job.$")
}

func (s *S) TestBuildErrorMessageWithNilResponse(c *C) {
	cli := Client{}
	err := errors.New("epic fail")
	c.Assert(cli.buildErrorMessage(err, nil), Equals, "epic fail")
}

func (s *S) TestBuildErrorMessageWithNilErrorAndNilResponse(c *C) {
	cli := Client{}
	c.Assert(cli.buildErrorMessage(nil, nil), Equals, "")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNilError(c *C) {
	cli := Client{}
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(nil, resp), Equals, "something went wrong")
}

func (s *S) TestBuildErrorMessageWithNonNilResponseAndNonNilError(c *C) {
	cli := Client{}
	err := errors.New("epic fail")
	body := strings.NewReader("something went wrong")
	resp := &http.Response{Body: ioutil.NopCloser(body)}
	c.Assert(cli.buildErrorMessage(err, resp), Equals, "epic fail")
}
