package megos

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestGetStdOutOfTask(t *testing.T) {
	setup()
	defer teardown()
	dir := "ct:1444473840000:0:metrics-collect-edgecast"
	stdOut := `Registered executor on 192.168.1.10
Starting task ct:1444564200000:0:metrics-collect-edgecast:
Forked command at 27595
sh -c 'echo "test"'
test
Command exited with status 0 (pid: 27595)`

	mux1.HandleFunc("/files/download.json", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"path": "ct:1444473840000:0:metrics-collect-edgecast/stdout",
		})

		fmt.Fprint(w, stdOut)
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
	body, err := client.GetStdOutOfTask(p, dir)

	if reflect.DeepEqual(body, []byte{}) {
		t.Error("Body is empty")
	}

	if !reflect.DeepEqual(body, []byte(stdOut)) {
		t.Errorf("Body is not as expected. Expected %s, got %s", stdOut, body)
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
}

func TestGetStdErrOfTask(t *testing.T) {
	setup()
	defer teardown()
	dir := "ct:1444473840000:0:metrics-collect-edgecast"
	stdOut := `I1011 13:50:04.852231 27541 exec.cpp:132] Version: 0.22.1
I1011 13:50:05.027619 27551 exec.cpp:206] Executor registered on slave 20150603-103119-2046951690-5050-24382-S1`

	mux1.HandleFunc("/files/download.json", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"path": "ct:1444473840000:0:metrics-collect-edgecast/stderr",
		})

		fmt.Fprint(w, stdOut)
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
	body, err := client.GetStdErrOfTask(p, dir)

	if reflect.DeepEqual(body, []byte{}) {
		t.Error("Body is empty")
	}

	if !reflect.DeepEqual(body, []byte(stdOut)) {
		t.Errorf("Body is not as expected. Expected %s, got %s", stdOut, body)
	}

	if err != nil {
		t.Errorf("Error is not nil. Expected nil, got %s", err)
	}
}
