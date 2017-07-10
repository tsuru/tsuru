package megos

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestGetMetricsSnapshot(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/metrics/snapshot", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("tests/master1.metrics.snapshot.json")
		fmt.Fprint(w, string(c))
	})

	role := "master"
	host := "127.0.0.1"
	port, _ := strconv.Atoi(strings.Split(server1.URL, ":")[2])
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	snapshot, err := client.GetMetricsSnapshot(parsedPid)
	if snapshot == nil {
		t.Error("Snapshot is nil. Expected not nil")
	}
	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
	if snapshot.SystemCpusTotal != 2 {
		t.Errorf("SystemCpusTotal is not right value. Expected %v, got %v", 2, snapshot.SystemCpusTotal)
	}
}

func TestGetMetricsSnapshotError(t *testing.T) {
	setup()
	defer teardown()

	mux1.HandleFunc("/metrics/snapshot", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		c := getContentOfFile("")
		fmt.Fprint(w, string(c))
	})

	role := "master"
	host := "127.0.0.1"
	port, _ := strconv.Atoi(strings.Split(server1.URL, ":")[2])
	pid := fmt.Sprintf("%s@%s:%d", role, host, port)
	parsedPid, _ := client.ParsePidInformation(pid)

	snapshot, err := client.GetMetricsSnapshot(parsedPid)
	if snapshot != nil {
		t.Errorf("Snapshot is not nil. Expected nil, get err is %v", err)
	}
	if err == nil {
		t.Errorf("Error is nil. Expected not nil")
	}
}
