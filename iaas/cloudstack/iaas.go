// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const (
	maxTry = 300
)

type cloudstackIaas struct{}

type NetInterface struct {
	IpAddress string
}

type CloudstackVirtualMachine struct {
	Nic []NetInterface
}

func (cs *CloudstackVirtualMachine) IsAvailable() bool {
	return true
}

func (cs *CloudstackVirtualMachine) GetAddress() string {
	return cs.Nic[0].IpAddress
}

func (i *cloudstackIaas) CreateVirtualMachine(params map[string]string) (iaas.Machine, error) {
	url, err := buildUrl("deployVirtualMachine", params)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var vmStatus map[string]string
	err = json.Unmarshal(body, &vmStatus)
	if err != nil {
		return nil, err
	}
	return waitVMIsCreated(vmStatus)
}

func buildUrl(command string, params map[string]string) (string, error) {
	apiKey, err := config.GetString("cloudstack:api-key")
	if err != nil {
		return "", err
	}
	secretKey, err := config.GetString("cloudstack:secret-key")
	if err != nil {
		return "", err
	}
	params["command"] = command
	params["response"] = "json"
	params["apiKey"] = apiKey
	var sorted_keys []string
	for k := range params {
		sorted_keys = append(sorted_keys, k)
	}
	sort.Strings(sorted_keys)
	var string_params []string
	for _, key := range sorted_keys {
		queryStringParam := fmt.Sprintf("%s=%s", key, url.QueryEscape(params[key]))
		string_params = append(string_params, queryStringParam)
	}
	queryString := strings.Join(string_params, "&")
	digest := hmac.New(sha1.New, []byte(secretKey))
	digest.Write([]byte(queryString))
	signature := base64.StdEncoding.EncodeToString(digest.Sum(nil))
	cloudstackUrl, err := config.GetString("cloudstack:url")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?%s&signature=%s", cloudstackUrl, queryString, signature), nil
}

func waitVMIsCreated(map[string]string) (*CloudstackVirtualMachine, error) {
	vmJson := `{"nic": [{"ipaddress": "0.0.0.0"}]}`
	vmJsonBuffer := bytes.NewBufferString(vmJson)
	var vm CloudstackVirtualMachine
	err := json.Unmarshal(vmJsonBuffer.Bytes(), &vm)
	if err != nil {
		return nil, err
	}
	return &vm, nil
}
