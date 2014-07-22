// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/tls"
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
	"time"
)

func init() {
	iaas.RegisterIaasProvider("cloudstack", &CloudstackIaaS{})
}

type ListVirtualMachinesResponse struct {
	VirtualMachine []VirtualMachineStruct `json:"virtualmachine"`
}
type VirtualMachineStruct struct {
	Nic []NicStruct `json:"nic"`
}
type NicStruct struct {
	IpAddress string `json:"ipaddress"`
}

type CloudstackIaaS struct{}

func (i *CloudstackIaaS) DeleteMachine(machine *iaas.Machine) error {
	return nil
}

func (i *CloudstackIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	userData, err := readUserData()
	if err != nil {
		return nil, err
	}
	params["userdata"] = userData
	url, err := buildUrl("deployVirtualMachine", params)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient().Get(url)
	if err != nil {
		return nil, err
	}
	var vmStatus map[string]map[string]string
	err = json.NewDecoder(resp.Body).Decode(&vmStatus)
	if err != nil {
		return nil, err
	}
	vmStatus["deployvirtualmachineresponse"]["projectid"] = params["projectid"]
	IpAddress, err := waitVMIsCreated(vmStatus)
	if err != nil {
		return nil, err
	}
	m := &iaas.Machine{
		Id:      IpAddress,
		Address: IpAddress,
		Status:  "running",
	}
	return m, nil
}

func readUserData() (string, error) {
	userDataUrl, _ := config.GetString("iaas:cloudstack:user-data")
	var userData string
	if userDataUrl == "" {
		userData = iaas.UserData
	} else {
		resp, err := http.Get(userDataUrl)
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Invalid user-data status code: %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		userData = string(body)
	}
	return base64.StdEncoding.EncodeToString([]byte(userData)), nil
}

func httpClient() *http.Client {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	return &http.Client{Transport: tr}
}

func buildUrl(command string, params map[string]string) (string, error) {
	apiKey, err := config.GetString("iaas:cloudstack:api-key")
	if err != nil {
		return "", err
	}
	secretKey, err := config.GetString("iaas:cloudstack:secret-key")
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
	digest.Write([]byte(strings.ToLower(queryString)))
	signature := base64.StdEncoding.EncodeToString(digest.Sum(nil))
	cloudstackUrl, err := config.GetString("iaas:cloudstack:url")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?%s&signature=%s", cloudstackUrl, queryString, url.QueryEscape(signature)), nil
}

func waitVMIsCreated(vmStatus map[string]map[string]string) (string, error) {
	jobData := vmStatus["deployvirtualmachineresponse"]
	count := 0
	maxTry := 300
	jobStatus := 0
	for jobStatus != 0 || count > maxTry {
		urlToJobCheck, _ := buildUrl("queryAsyncJobResult", map[string]string{"jobid": jobData["jobid"]})
		resp, err := httpClient().Get(urlToJobCheck)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		var jobCheckStatus map[string]interface{}
		err = json.Unmarshal(body, &jobCheckStatus)
		if err != nil {
			return "", err
		}
		jobStatus = jobCheckStatus["jobstatus"].(int)
		count = count + 1
		time.Sleep(time.Second)
	}
	urlToGetMachineInfo, _ := buildUrl("listVirtualMachines", map[string]string{"id": jobData["id"], "projectid": jobData["projectid"]})
	resp, err := httpClient().Get(urlToGetMachineInfo)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var machineInfo map[string]ListVirtualMachinesResponse
	err = json.Unmarshal(body, &machineInfo)
	if err != nil {
		return "", err
	}
	return machineInfo["listvirtualmachinesresponse"].VirtualMachine[0].Nic[0].IpAddress, nil
}
