package megos

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

var (
	// client is the Megos client being tested.
	client *Client

	// master is a list of (faked) mesos master nodes
	master []*url.URL

	// mux1 is the HTTP request multiplexer used with the test server.
	mux1 *http.ServeMux

	// server1 is a test HTTP server used to provide mock API responses.
	server1 *httptest.Server

	// mux2 is the HTTP request multiplexer used with the test server.
	mux2 *http.ServeMux

	// server2 is a test HTTP server used to provide mock API responses.
	server2 *httptest.Server

	// mux3 is the HTTP request multiplexer used with the test server.
	mux3 *http.ServeMux

	// server3 is a test HTTP server used to provide mock API responses.
	server3 *httptest.Server
)

type values map[string]string

// setup sets up a test HTTP server along with a megos.Client that is configured to talk to that test server.
// Tests should register handlers on mux which provide mock responses for the http call being tested.
func setup() {
	// test server: 1
	mux1 = http.NewServeMux()
	server1 = httptest.NewServer(mux1)

	// test server: 2
	mux2 = http.NewServeMux()
	server2 = httptest.NewServer(mux2)

	// test server: 3
	mux3 = http.NewServeMux()
	server3 = httptest.NewServer(mux3)

	m1, _ := url.Parse(server1.URL)
	m2, _ := url.Parse(server2.URL)
	m3, _ := url.Parse(server3.URL)
	master = []*url.URL{m1, m2, m3}

	client = NewClient(master, nil)
}

// teardown closes the test HTTP server.
func teardown() {
	server1.Close()
	server2.Close()
	server3.Close()
}

// testMethod is a utility function to test the request method provided in want
func testMethod(t *testing.T, r *http.Request, want string) {
	if got := r.Method; got != want {
		t.Errorf("Request method: %v, want %v", got, want)
	}
}

// testFormValues is a utility method to test the query values given in values
func testFormValues(t *testing.T, r *http.Request, values values) {
	want := url.Values{}
	for k, v := range values {
		want.Add(k, v)
	}

	r.ParseForm()
	if got := r.Form; !reflect.DeepEqual(got, want) {
		t.Errorf("Request parameters: %v, want %v", got, want)
	}
}

// getContentOfFile is a utility method to open and return the content of fileName
func getContentOfFile(fileName string) []byte {
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		return []byte{}
	}

	return content
}

func TestNewClient(t *testing.T) {
	setup()
	defer teardown()

	if client == nil {
		t.Error("Megos client is nil. Expected megos.Client structure")
	}

	if !reflect.DeepEqual(client.Master, master) {
		t.Error("Megos master are not the same as initialized.")
	}
}

func TestParsePidInformation_WithPort(t *testing.T) {
	role := "master"
	host := "192.168.99.100"
	port := 5555
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	if parsedPid.Role != role {
		t.Errorf("Role is not equal. Expected %s, got %s", role, parsedPid.Role)
	}
	if parsedPid.Host != host {
		t.Errorf("Host is not equal. Expected %s, got %s", host, parsedPid.Host)
	}
	if parsedPid.Port != port {
		t.Errorf("Port is not equal. Expected %d, got %d", port, parsedPid.Port)
	}
}

func TestParsePidInformation_WithoutPort(t *testing.T) {
	role := "master"
	host := "192.168.99.100"
	port := 5050
	pid := fmt.Sprintf("%s@%s", role, host)
	parsedPid, _ := client.ParsePidInformation(pid)

	if parsedPid.Role != role {
		t.Errorf("Role is not equal. Expected %s, got %s", role, parsedPid.Role)
	}
	if parsedPid.Host != host {
		t.Errorf("Host is not equal. Expected %s, got %s", host, parsedPid.Host)
	}
	if parsedPid.Port != port {
		t.Errorf("Port is not equal. Expected %d, got %d", port, parsedPid.Port)
	}
}

func TestParsePidInformation_String(t *testing.T) {
	role := "master"
	host := "192.168.99.100"
	port := 5555
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	if s := parsedPid.String(); s != pid {
		t.Errorf("Stringer of pid is not equal. Expected %s, got %s", pid, s)
	}
}

func TestDetermineLeader_NoNodeOnline(t *testing.T) {
	setup()
	defer teardown()

	v, _ := url.Parse("http://not-existing.example.org/")
	client.Master = []*url.URL{v, v, v}
	p, err := client.DetermineLeader()
	if p != nil {
		t.Errorf("Pid is not nil. Expected nil, got %+v", p)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error (No master online.).")
	}
}

func TestDetermineLeader(t *testing.T) {
	setup()
	defer teardown()
	expected := "master@192.168.99.100:5050"

	mux1.HandleFunc("/master/state", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.state.json")
		fmt.Fprint(w, string(c))
	})

	p, err := client.DetermineLeader()
	if p == nil {
		t.Error("Pid is nil. Expected not nil")
	}

	if s := p.String(); s != expected {
		t.Errorf("Wrong pid. Extected %s, got %s", expected, s)
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
}
