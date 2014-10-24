// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openstack

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	openstack "git.openstack.org/stackforge/golang-client.git/identity/v2"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
)

func init() {
	iaas.RegisterIaasProvider("openstack", &OpenstackIaaS{})
}

type OpenstackIaaS struct {
	iaasName string
}

var zeroByte = &([]byte{})

func (i *OpenstackIaaS) getConfigString(name string) (string, error) {
	val, err := config.GetString(fmt.Sprintf("iaas:openstack:%s:%s", i.iaasName, name))
	if err != nil {
		val, err = config.GetString(fmt.Sprintf("iaas:openstack:%s", name))
	}
	return val, err
}

func (i *OpenstackIaaS) Clone(name string) iaas.IaaS {
	clone := *i
	clone.iaasName = name
	return &clone
}

func (i *OpenstackIaaS) Describe() string {
	return `Openstack IaaS required params:
  api_endpoint=<keystone url and port>      Your openstack authentication endpoint
  Username=<username>                       Your Username
	Password=<password>                       Your Password
	ProjectName=<ProjectName>                 Your ProjectName
  Imageid=<templateid>                      Your template uuid
  zonename=<zonename>                       Your zone name
`
}

func (i *OpenstackIaaS) createToken() (*openstack.Auth, error) {
	api_endpoint, err := i.getConfigString("api_endpoint")
	if err != nil {
		panic(err)
	}
	ProjectName, err := i.getConfigString("ProjectName")
	if err != nil {
		panic(err)
	}
	Username, err := i.getConfigString("Username")
	if err != nil {
		panic(err)
	}

	Password, err := i.getConfigString("Password")
	if err != nil {
		panic(err)
	}
	auth := openstack.Auth{}
	auth, err = openstack.AuthUserNameTenantName(api_endpoint, Username, Password, ProjectName)
	if err != nil {
		panicString := fmt.Sprint("There was an authenticating error:", err)
		panic(panicString)
	}
	return &auth, err
}

func (i *OpenstackIaaS) vmName() string {
	size := 8
	rb := make([]byte, size)
	_, err := rand.Read(rb)
	if err != nil {
		return ""
	}
	rs := base64.URLEncoding.EncodeToString(rb)
	rs = strings.Replace(rs, "=", "", -1)

	return "tsuru-" + rs

}

func (i *OpenstackIaaS) vmProvision(url string, params map[string]string, token string, result interface{}) error {
	computeUrl := url
	h := []string{"X-Auth-Project-Id"}
	h = append(h, params["ProjectName"])
	h = append(h, "Content-Type")
	h = append(h, "application/json")
	h = append(h, "X-Auth-Token")
	h = append(h, token)

	var contents map[string]string
	var err error
	contents = make(map[string]string)
	contents["name"] = i.vmName()
	contents["imageRef"], err = i.getConfigString("ImageID")
	contents["flavorRef"], err = i.getConfigString("FlavorID")
	contents["availability_zone"], err = i.getConfigString("ZoneName")

	vms := preInfo{contents}
	jsonString, err := json.Marshal(vms)
	if err != nil {
		panic(err)
	}

	resp, err := CallAPI("POST", computeUrl, jsonString, h)

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", "create", resp.StatusCode, string(body))
	}
	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return fmt.Errorf("Unexpected result data for %s command: %s - Body: %s", "create", err.Error(), string(body))
		}
	}
	return nil
}

func (i *OpenstackIaaS) vmInfo(url string, token string, serverId string, result interface{}) error {
	computeUrl := url + "/" + serverId + "?format=json"
	h := []string{"X-Auth-Token"}
	h = append(h, token)

	resp, err := CallAPI("GET", computeUrl, *zeroByte, h)

	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", "create", resp.StatusCode, string(body))
	}
	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return fmt.Errorf("Unexpected result data for %s command: %s - Body: %s", "create", err.Error(), string(body))
		}
	}
	return nil
}

func (i *OpenstackIaaS) vmList(url string, token string, params map[string]string, result interface{}) error {
	computeUrl := url + "/" + params["Id"] + "?format=json"
	h := []string{"X-Auth-Token"}
	h = append(h, token)

	var req *http.Request
	req, err := http.NewRequest("GET", computeUrl, nil)
	req.Header.Set(h[0], h[1])
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", "create", resp.StatusCode, string(body))
	}
	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return fmt.Errorf("Unexpected result data for %s command: %s - Body: %s", "create", err.Error(), string(body))
		}
	}
	return nil
}

func (i *OpenstackIaaS) getVMId(url string, token string, params map[string]string, result interface{}) error {
	computeUrl := url + "?name=" + params["virtualmachineName"]
	h := []string{"X-Auth-Token"}
	h = append(h, token)

	resp, err := CallAPI("GET", computeUrl, *zeroByte, h)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", "create", resp.StatusCode, string(body))
	}
	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return fmt.Errorf("Unexpected result data for %s command: %s - Body: %s", "create", err.Error(), string(body))
		}
	}
	return nil

}

func (i *OpenstackIaaS) DeleteMachine(machine *iaas.Machine) error {

	openstackAuth, err := i.createToken()
	url, err := i.makeTenantURI(openstackAuth)
	var dat map[string]interface{}
	err = i.getVMId(url, openstackAuth.Access.Token.Id, ApiParams{"virtualmachineName": machine.Id, "projectid": machine.CreationParams["projecti"]}, &dat)
	object := dat["servers"].([]interface{})
	id := object[0].(map[string]interface{})
	vmId := id["id"].(string)

	err = i.vmDelete(url, openstackAuth.Access.Token.Id, ApiParams{
		"vmId":      vmId,
		"projectid": machine.CreationParams["projectid"]})

	if err != nil {
		return err
	}
	return nil
}

func (i *OpenstackIaaS) vmDelete(url string, token string, params map[string]string) error {

	computeUrl := url + "/" + params["vmId"]
	h := []string{"X-Auth-Token"}
	h = append(h, token)
	resp, err := CallAPI("DELETE", computeUrl, *zeroByte, h)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", "create", resp.StatusCode, string(body))
	}
	return nil

}

func CallAPI(method, url string, content []byte, h []string) (*http.Response, error) {
	var req *http.Request
	var err error
	req, err = http.NewRequest(method, url, nil)

	if len(content) > 0 {
		req.Body = readCloser{bytes.NewReader(content)}
	}
	req, err = http.NewRequest(method, url, bytes.NewBuffer(content))
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(h)-1; i = i + 2 {
		req.Header.Set(h[i], h[i+1])
	}
	return (new(http.Client)).Do(req)
}

type readCloser struct {
	io.Reader
}

func (readCloser) Close() error {
	//cannot put this func inside CallAPI; golang disallow nested func
	return nil
}

func (i *OpenstackIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {

	openstackAuth, err := i.createToken()
	url, err := i.makeTenantURI(openstackAuth)

	var vmStatus DeployVMresult
	err = i.vmProvision(url, params, openstackAuth.Access.Token.Id, &vmStatus)
	if err != nil {
		panic(err)
	}
	IpAddress, Name, err := i.waitVMIsCreated(url, openstackAuth.Access.Token.Id,
		vmStatus.DeployVMresult.ID, params["projectName"])

	if err != nil {
		return nil, err
	}
	m := &iaas.Machine{
		Id:      Name,
		Address: IpAddress,
		Status:  "running",
	}
	return m, nil
}

func (i *OpenstackIaaS) makeTenantURI(auth *openstack.Auth) (string, error) {

	computeUrl := ""
	for _, svc := range auth.Access.ServiceCatalog {
		if svc.Type == "compute" {
			computeUrl = svc.Endpoints[0].PublicURL + "/"
			break
		}
	}
	return computeUrl + "servers", nil
}

func (i *OpenstackIaaS) waitForAsyncJob(url string, token string, machineId string) (QueryJobResult, error) {
	count := 0
	maxTry := 300
	var jobResponse QueryJobResult
	for count < maxTry {
		err := i.vmList(url, token, ApiParams{"Id": machineId}, &jobResponse)
		if err != nil {
			return jobResponse, err
		}
		if jobResponse.QueryJobResult.JobStatus != JOB_STATUS_IN_BUILD {
			if jobResponse.QueryJobResult.JobStatus == JOB_STATUS_FAILED {
				return jobResponse, fmt.Errorf("Job failed to complete: %#v", jobResponse.QueryJobResult.JobStatus)
			}
			return jobResponse, nil
		}
		count = count + 1
		time.Sleep(time.Second)
	}
	return jobResponse, fmt.Errorf("Maximum number of retries waiting for job %q", machineId)
}

func (i *OpenstackIaaS) waitVMIsCreated(url string, token string, machineId string, projectId string) (string, string, error) {

	_, err := i.waitForAsyncJob(url, token, machineId)
	if err != nil {
		return "", "", err
	}
	var machineInfo GetVMInfo
	err = i.vmInfo(url, token, machineId, &machineInfo)
	if err != nil {
		return "", "", err
	}

	machineName := machineInfo.GetVMInfo.Name
	az := machineInfo.GetVMInfo.Az
	object := machineInfo.GetVMInfo.Info.(map[string]interface{})
	network := object[az].([]interface{})
	addr := network[0].(map[string]interface{})
	ip := addr["addr"].(string)

	return ip, machineName, nil
}
