// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tsuru/tsuru/app"
)

func clientWithTimeout(timeout time.Duration) *http.Client {
	dialTimeout := func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, timeout)
	}
	transport := http.Transport{
		Dial: dialTimeout,
	}
	return &http.Client{
		Transport: &transport,
	}
}

var timeoutHttpClient = clientWithTimeout(5 * time.Second)

func runHealthcheck(cont *container, w io.Writer, maxWaitTime time.Duration) error {
	dbApp, err := app.GetByName(cont.AppName)
	if err != nil {
		return nil
	}
	hc, ok := dbApp.CustomData["healthcheck"].(map[string]interface{})
	if !ok {
		return nil
	}
	path, _ := hc["path"].(string)
	if path == "" {
		return nil
	}
	path = strings.TrimSpace(strings.TrimLeft(path, "/"))
	method, _ := hc["method"].(string)
	if method == "" {
		method = "get"
	}
	method = strings.ToUpper(method)
	var status int
	switch val := hc["status"].(type) {
	case int:
		status = val
	case float64:
		status = int(val)
	default:
	}
	match, _ := hc["match"].(string)
	if status == 0 && match == "" {
		status = 200
	}
	var matchRE *regexp.Regexp
	if match != "" {
		match = "(?s)" + match
		matchRE, err = regexp.Compile(match)
		if err != nil {
			return err
		}
	}
	sleepTime := 3 * time.Second
	startedTime := time.Now()
	url := fmt.Sprintf("http://%s:%s/%s", cont.HostAddr, cont.HostPort, path)
	for {
		var lastError error = nil
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return err
		}
		rsp, err := timeoutHttpClient.Do(req)
		if err != nil {
			lastError = fmt.Errorf("healthcheck fail(%s): %s", cont.shortID(), err.Error())
		} else {
			if status != 0 && rsp.StatusCode != status {
				return fmt.Errorf("healthcheck fail(%s): wrong status code, expected %d, got: %d", cont.shortID(), status, rsp.StatusCode)
			} else if matchRE != nil {
				result, err := ioutil.ReadAll(rsp.Body)
				if err != nil {
					return err
				}
				if !matchRE.Match(result) {
					return fmt.Errorf("healthcheck fail(%s): unexpected result, expected %q, got: %s", cont.shortID(), match, string(result))
				}
			}
		}
		if lastError == nil {
			fmt.Fprintf(w, " ---> healthcheck successful(%s)\n", cont.shortID())
			return nil
		}
		if time.Now().Sub(startedTime) > maxWaitTime {
			return lastError
		}
		fmt.Fprintf(w, " ---> %s. Trying again in %ds\n", lastError.Error(), sleepTime/time.Second)
		time.Sleep(sleepTime)
	}
	return nil
}
