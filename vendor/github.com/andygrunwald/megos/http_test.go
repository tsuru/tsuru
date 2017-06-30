package megos

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

func TestGetHTTPResponse(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, "Test success")
	})
	r, err := client.GetHTTPResponse(client.Master[0])

	if r == nil {
		t.Errorf("Response is nil. Expected valid http.Response object, got nil")
	}

	if r.StatusCode != 200 {
		t.Errorf("Response Statuscode is not 200. Expected 200, got %d", r.StatusCode)
	}

	if err != nil {
		t.Errorf("err is not nil. Expected nil, got %s", err)
	}
}

func TestGetHTTPResponse_WithError(t *testing.T) {
	setup()
	defer teardown()

	u, _ := url.Parse("https://not-existing.example.org/")
	r, err := client.GetHTTPResponse(u)

	if r != nil {
		t.Errorf("Response is not nil. Expected nil, got %+v", r)
	}

	if err == nil {
		t.Errorf("err is nil. Expected error, got nil")
	}
}

func TestGetBodyOfHTTPResponse(t *testing.T) {
	setup()
	defer teardown()
	body := "Test success"
	byteBody := []byte(body)

	mux1.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, body)
	})
	b, err := client.GetBodyOfHTTPResponse(client.Master[0])

	if !reflect.DeepEqual(b, byteBody) {
		t.Errorf("Response body is not equal. Expected %+v, got %+v", byteBody, b)
	}

	if err != nil {
		t.Errorf("err is not nil. Expected nil, got %s", err)
	}
}

func TestGetBodyOfHTTPResponse_WithError(t *testing.T) {
	setup()
	defer teardown()
	expected := []byte{}

	u, _ := url.Parse("https://not-existing.example.org/")
	b, err := client.GetBodyOfHTTPResponse(u)

	if !reflect.DeepEqual(b, expected) {
		t.Errorf("Response body is not equal. Expected %+v, got %+v", expected, b)
	}

	if err == nil {
		t.Errorf("err is nil. Expected error, got nil")
	}
}

func TestGetHTTPResponseFromCluster_NoMaster(t *testing.T) {
	setup()
	defer teardown()

	testFunc := func(u url.URL) url.URL {
		v, _ := url.Parse("https://not-existing.example.org/")
		return *v
	}
	resp, err := client.GetHTTPResponseFromCluster(testFunc)
	if resp != nil {
		t.Errorf("Response is not nil. Expected nil, got %+v", resp)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error (No master online.).")
	}
}

func TestGetHTTPResponseFromCluster(t *testing.T) {
	setup()
	defer teardown()

	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, "content")
	})

	i := 0
	testFunc := func(u url.URL) url.URL {
		i++
		// The first node should fail
		// With this we can ensure that more than one node is requested in failure case
		if i == 1 {
			v, _ := url.Parse("http://not-existing.example.org/")
			return *v
		}
		u.Path = "master/state"
		return u
	}
	resp, err := client.GetHTTPResponseFromCluster(testFunc)
	if resp == nil {
		t.Errorf("Response is nil. Expected valid response, got nil")
	}

	if resp.StatusCode != 200 {
		t.Errorf("Response status code is not 200. Expected 200, got %d", resp.StatusCode)
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s.", err)
	}
}

func TestGetHTTPResponseFromLeader_NoLeader(t *testing.T) {
	setup()
	defer teardown()

	testFunc := func(p Pid) url.URL {
		return url.URL{}
	}
	resp, err := client.GetHTTPResponseFromLeader(testFunc)
	if resp != nil {
		t.Errorf("Response is not nil. Expected nil, got %+v", resp)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error (No leader set.).")
	}
}

func TestGetHTTPResponseFromLeader(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, "content")
	})

	client.Leader = &Pid{}
	testFunc := func(p Pid) url.URL {
		u, _ := url.Parse(server1.URL)
		return *u
	}
	resp, err := client.GetHTTPResponseFromLeader(testFunc)
	if resp == nil {
		t.Errorf("Response is nil. Expected valid response, got nil")
	}

	if resp.StatusCode != 200 {
		t.Errorf("Response status code is not 200. Expected 200, got %d", resp.StatusCode)
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s.", err)
	}
}
