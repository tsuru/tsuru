// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth/saml"
	tsuruNet "github.com/tsuru/tsuru/net"
)

const formPostPage = `<!DOCTYPE html>
<html>
<head>
	<style>
	body {
		display: none;
	}
	</style>
</head>
<body onload="document.frm.submit()">
	<form method="POST" name="frm" action="{{.url}}">
		<input type="hidden" name="SAMLRequest" value="{{.saml_request}}" />
		<input type="submit" value="Go to login" />
	</form>
</body>
</html>
`

const successSamlCallBackMarkup = `
	<script>window.close();</script>
	<h1>Login Successful!</h1>
	<p>You can close this window now.</p>
`

const errorSamlCallBackMarkup = `
	<h1>Login Failed!</h1>
	<pre>%s</pre>
`

func SamlCallbackSuccessMessage() string {
	return successSamlCallBackMarkup
}

func SamlCallbackFailureMessage() string {
	return errorSamlCallBackMarkup
}

func samlRequestId(schemeData map[string]string) string {
	return schemeData["request_id"]
}

//Return timeout in seconds
func samlRequestTimeout(schemeData map[string]string) int {
	p := schemeData["request_timeout"]
	timeout, _ := strconv.Atoi(p)
	return timeout
}

func samlPreLogin(schemeData map[string]string, finish chan bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		t := template.New("saml")
		t, err := t.Parse(formPostPage)
		if err != nil {
			page := fmt.Sprintf(errorSamlCallBackMarkup, err.Error())
			w.Header().Add("Content-Type", "text/html")
			w.Write([]byte(page))
		} else {
			t.Execute(w, schemeData)
		}
	}
}

func requestToken(schemeData map[string]string) (string, error) {
	maxRetries := samlRequestTimeout(schemeData) - 7
	time.Sleep(5 * time.Second)
	id := samlRequestId(schemeData)
	v := url.Values{}
	v.Set("request_id", id)
	for count := 0; count <= maxRetries; count += 2 {
		u, err := GetURL("/auth/login")
		if err != nil {
			return "", errors.Wrap(err, "Error in GetURL")
		}
		resp, err := tsuruNet.Dial15Full300Client.Post(u, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "Error during login post")
		}
		defer resp.Body.Close()
		result, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "Error reading body")
		}
		if strings.TrimSpace(string(result)) == saml.ErrRequestWaitingForCredentials.Message {
			if count < maxRetries {
				time.Sleep(2 * time.Second)
			}
			continue
		}
		data := make(map[string]interface{})
		if err = json.Unmarshal(result, &data); err != nil {
			return "", errors.Errorf("API response: %s", result)
		}
		return data["token"].(string), nil
	}
	// finish when timeout
	return "", saml.ErrRequestWaitingForCredentials
}

func (c *login) samlLogin(context *Context, client *Client) error {
	schemeData := c.getScheme().Data
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return err
	}
	finish := make(chan bool)
	preLoginURL := fmt.Sprintf("http://localhost:%s/", port)
	http.HandleFunc("/", samlPreLogin(schemeData, finish))
	server := &http.Server{}
	go server.Serve(l)
	if err = open(preLoginURL); err != nil {
		fmt.Fprintln(context.Stdout, "Failed to start your browser.")
		fmt.Fprintf(context.Stdout, "Please open the following URL in your browser: %s\n", preLoginURL)
	}
	<-finish
	token, err := requestToken(schemeData)
	switch err {
	case nil:
		writeToken(token)
		fmt.Fprintln(context.Stdout, "\nSuccessfully logged in!")
	case saml.ErrRequestWaitingForCredentials:
		fmt.Fprintln(context.Stdout, "\nLogin failed! Timeout waiting for credentials from IDP, please try again.")
	default:
		fmt.Fprintln(context.Stdout, "\nLogin failed for some reason, please try again: "+err.Error())
	}
	return nil
}
