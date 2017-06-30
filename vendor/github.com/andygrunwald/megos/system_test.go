package megos

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestGetSystemFromPid(t *testing.T) {
	setup()
	defer teardown()
	expected := 0.13

	mux1.HandleFunc("/system/stats.json", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.system.stats.json")
		fmt.Fprint(w, string(c))
	})

	role := "master"
	host := "127.0.0.1"
	port, _ := strconv.Atoi(strings.Split(server1.URL, ":")[2])
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	system, err := client.GetSystemFromPid(parsedPid)
	if system == nil {
		t.Error("System is nil. Expected not nil")
	}
	if system.AvgLoad15min != expected {
		t.Errorf("Avg Load 15min is %v, Expected 0.13", system.AvgLoad15min)
	}
	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
}

func TestGetSystemFromPidError(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/system/stats.json", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("")
		fmt.Fprint(w, string(c))
	})

	role := "master"
	host := "127.0.0.1"
	port, _ := strconv.Atoi(strings.Split(server1.URL, ":")[2])
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	system, err := client.GetSystemFromPid(parsedPid)
	if system != nil {
		t.Errorf("System is Not nil. Expected nil, got error %v.", err)
	}
	if err == nil {
		t.Errorf("Error is nil. Expected Not nil.")
	}
}
