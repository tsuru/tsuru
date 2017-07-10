package megos

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestGetURLForStateFile(t *testing.T) {
	setup()
	defer teardown()

	masterNode := client.Master[1]
	u := client.GetURLForStateFile(*masterNode)

	ms := masterNode.String() + "/master/state"
	if us := u.String(); ms != us {
		t.Errorf("URLs not the same. Expected %s, got %s", ms, us)
	}
}

func TestGetURLForStateFilePid(t *testing.T) {
	setup()
	defer teardown()

	p := Pid{
		Role: "master",
		Host: "192.168.1.6",
		Port: 5050,
	}

	u := client.GetURLForStateFilePid(p)

	ps := fmt.Sprintf("http://%s:%d/master/state", p.Host, p.Port)
	if us := u.String(); ps != us {
		t.Errorf("URLs not the same. Expected %s, got %s", ps, us)
	}
}

func TestGetStateFromCluster_NoNodesOnline(t *testing.T) {
	setup()
	defer teardown()

	u, _ := url.Parse("http://not-existing.example.org/")
	client.Master = []*url.URL{u, u, u}
	state, err := client.GetStateFromCluster()

	if state != nil {
		t.Errorf("State is not nil. Expected nil. Got %+v", state)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error.")
	}
}

func TestGetStateFromCluster(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/master/state", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.state.json")
		fmt.Fprint(w, string(c))
	})

	state, err := client.GetStateFromCluster()

	if state == nil {
		t.Error("State is nil. Expected valid state struct")
	}

	// Check if some random samples are matching with dummy content in master1.state.json
	if state != nil && (state.Cluster != "docker-compose" || state.Version != "0.28.1" || state.Flags.LogDir != "/var/log/mesos" || state.Flags.ZKSessionTimeout != "10secs") {
		t.Error("Random samples are not matching with test mock data.")
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s.", err)
	}
}

func TestGetStateFromLeader_NoLeader(t *testing.T) {
	setup()
	defer teardown()

	state, err := client.GetStateFromLeader()

	if state != nil {
		t.Errorf("State is not nil. Expected nil. Got %+v", state)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error.")
	}
}

func TestGetStateFromLeader_NoLeaderOnline(t *testing.T) {
	setup()
	defer teardown()

	client.Leader = &Pid{
		Role: "master",
		Host: "not-existing.example.org",
	}
	state, err := client.GetStateFromLeader()

	if state != nil {
		t.Errorf("State is not nil. Expected nil. Got %+v", state)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error.")
	}
}

func TestGetStateFromLeader(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/master/state", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.state.json")
		fmt.Fprint(w, string(c))
	})

	u, _ := url.Parse(server1.URL)
	parts := strings.Split(u.Host, ":")
	host, port := parts[0], parts[1]
	portI, _ := strconv.Atoi(port)
	client.Leader = &Pid{
		Role: "master",
		Host: host,
		Port: portI,
	}
	state, err := client.GetStateFromLeader()

	if state == nil {
		t.Error("State is nil. Expected valid state struct")
	}

	// Check if some random samples are matching with dummy content in master1.state.json
	if state != nil && (state.Cluster != "docker-compose" || state.Version != "0.28.1" || state.Flags.LogDir != "/var/log/mesos" || state.Flags.ZKSessionTimeout != "10secs") {
		t.Error("Random samples are not matching with test mock data.")
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
}

func TestGetStateFromPid_PidOffline(t *testing.T) {
	setup()
	defer teardown()

	p := &Pid{
		Role: "master",
		Host: "not-existing.example.org",
	}
	state, err := client.GetStateFromPid(p)

	if state != nil {
		t.Errorf("State is not nil. Expected nil. Got %+v", state)
	}

	if err == nil {
		t.Error("Error is nil. Expected an error.")
	}
}

func TestGetStateFromPid(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/master/state", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.state.json")
		fmt.Fprint(w, string(c))
	})

	u, _ := url.Parse(server1.URL)
	parts := strings.Split(u.Host, ":")
	host, port := parts[0], parts[1]
	portI, _ := strconv.Atoi(port)
	p := &Pid{
		Role: "master",
		Host: host,
		Port: portI,
	}
	state, err := client.GetStateFromPid(p)

	if state == nil {
		t.Error("State is nil. Expected valid state struct")
	}

	// Check if some random samples are matching with dummy content in master1.state.json
	if state != nil && (state.Cluster != "docker-compose" || state.Version != "0.28.1" || state.Flags.LogDir != "/var/log/mesos" || state.Flags.ZKSessionTimeout != "10secs") {
		t.Error("Random samples are not matching with test mock data.")
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s.", err)
	}
}
