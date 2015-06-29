// Copyright 2015 tsuru authors. All rights reserved.
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

	"github.com/tsuru/config"
)

func clientWithTimeout(dialTimeout time.Duration) *http.Client {
	transport := http.Transport{
		Dial: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: dialTimeout,
	}
	return &http.Client{
		Transport: &transport,
		Timeout:   1 * time.Minute,
	}
}

var timeoutHttpClient = clientWithTimeout(5 * time.Second)

func runHealthcheck(cont *container, w io.Writer) error {
	yamlData, err := getImageTsuruYamlData(cont.Image)
	if err != nil {
		return err
	}
	path := yamlData.Healthcheck.Path
	method := yamlData.Healthcheck.Method
	match := yamlData.Healthcheck.Match
	status := yamlData.Healthcheck.Status
	allowedFailures := yamlData.Healthcheck.AllowedFailures
	if path == "" {
		return nil
	}
	path = strings.TrimSpace(strings.TrimLeft(path, "/"))
	if method == "" {
		method = "get"
	}
	method = strings.ToUpper(method)
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
	maxWaitTime, _ := config.GetInt("docker:healthcheck:max-time")
	if maxWaitTime == 0 {
		maxWaitTime = 120
	}
	maxWaitTime = maxWaitTime * int(time.Second)
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
			defer rsp.Body.Close()
			if status != 0 && rsp.StatusCode != status {
				lastError = fmt.Errorf("healthcheck fail(%s): wrong status code, expected %d, got: %d", cont.shortID(), status, rsp.StatusCode)
			} else if matchRE != nil {
				result, err := ioutil.ReadAll(rsp.Body)

				if err != nil {
					lastError = err
				}
				if !matchRE.Match(result) {
					lastError = fmt.Errorf("healthcheck fail(%s): unexpected result, expected %q, got: %s", cont.shortID(), match, string(result))
				}
			}
			if lastError != nil {
				if allowedFailures == 0 {
					return lastError
				}
				allowedFailures--
			}
		}
		if lastError == nil {
			fmt.Fprintf(w, " ---> healthcheck successful(%s)\n", cont.shortID())
			return nil
		}
		if time.Now().Sub(startedTime) > time.Duration(maxWaitTime) {
			return lastError
		}
		fmt.Fprintf(w, " ---> %s. Trying again in %ds\n", lastError.Error(), sleepTime/time.Second)
		time.Sleep(sleepTime)
	}
}
