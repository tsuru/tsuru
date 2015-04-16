// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cloudstack

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/monsterqueue"
	"github.com/tsuru/tsuru/hc"
	"github.com/tsuru/tsuru/iaas"
	"github.com/tsuru/tsuru/queue"
)

func init() {
	iaas.RegisterIaasProvider("cloudstack", newCloudstackIaaS)
	hc.AddChecker("CloudStack", iaas.BuildHealthCheck("cloudstack"))
}

type CloudstackIaaS struct {
	base iaas.UserDataIaaS
}

func newCloudstackIaaS(name string) iaas.IaaS {
	return &CloudstackIaaS{base: iaas.UserDataIaaS{NamedIaaS: iaas.NamedIaaS{BaseIaaSName: "cloudstack", IaaSName: name}}}
}

func (i *CloudstackIaaS) Describe() string {
	return `Cloudstack IaaS required params:
  networkids=<networkids>                   Your network uuid
  templateid=<templateid>                   Your template uuid
  serviceofferingid=<serviceofferingid>     Your service offering uuid
  zoneid=<zoneid>                           Your zone uuid

Further params will also be sent to cloudstack's deployVirtualMachine command.
`
}

func (i *CloudstackIaaS) HealthCheck() error {
	var resp ListZonesResponse
	err := i.do("listZones", map[string]string{}, &resp)
	if err != nil {
		return err
	}
	if resp.ListZonesResponse.Count < 1 {
		name := i.base.IaaSName
		if name == "" {
			name = i.base.BaseIaaSName
		}
		return fmt.Errorf("%q - not enough zones available, want at least 1, got %d", name, resp.ListZonesResponse.Count)
	}
	return nil
}

func (i *CloudstackIaaS) Initialize() error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	err = q.RegisterTask(&machineCreate{iaas: i})
	if err != nil {
		return err
	}
	return q.RegisterTask(&machineDelete{iaas: i})
}

func validateParams(params map[string]string) error {
	mandatory := []string{"networkids", "templateid", "serviceofferingid", "zoneid"}
	for _, p := range mandatory {
		_, isPresent := params[p]
		if !isPresent {
			return fmt.Errorf("param %q is mandatory", p)
		}
	}
	return nil
}

func (i *CloudstackIaaS) taskName(name string) string {
	return fmt.Sprintf("%s-%s", name, i.base.IaaSName)
}

func (i *CloudstackIaaS) do(cmd string, params map[string]string, result interface{}) error {
	url, err := i.buildUrl(cmd, params)
	if err != nil {
		return err
	}
	client := http.DefaultClient
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response code for %s command %d: %s", cmd, resp.StatusCode, string(body))
	}
	if result != nil {
		err = json.Unmarshal(body, result)
		if err != nil {
			return fmt.Errorf("Unexpected result data for %s command: %s - Body: %s", cmd, err.Error(), string(body))
		}
	}
	return nil
}

func (i *CloudstackIaaS) DeleteMachine(machine *iaas.Machine) error {
	q, err := queue.Queue()
	if err != nil {
		return err
	}
	rawWait, _ := i.base.GetConfigString("wait-timeout")
	maxWaitTime, _ := strconv.Atoi(rawWait)
	if maxWaitTime == 0 {
		maxWaitTime = 300
	}
	waitDuration := time.Duration(maxWaitTime) * time.Second
	job, err := q.EnqueueWait(i.taskName(machineDeleteTaskName), monsterqueue.JobParams{
		"vmId":      machine.Id,
		"projectId": machine.CreationParams["projectid"],
	}, waitDuration)
	if err != nil {
		if err == monsterqueue.ErrQueueWaitTimeout {
			return fmt.Errorf("cloudstack: time out after %v waiting for instance %s to be destroyed", waitDuration, machine.Id)
		}
		return err
	}
	_, err = job.Result()
	return err
}

func (i *CloudstackIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	err := validateParams(params)
	if err != nil {
		return nil, err
	}
	userData, err := i.base.ReadUserData()
	if err != nil {
		return nil, err
	}
	q, err := queue.Queue()
	if err != nil {
		return nil, err
	}
	paramsCopy := make(map[string]string)
	for k, v := range params {
		paramsCopy[k] = v
	}
	if userData != "" {
		paramsCopy["userdata"] = userData
	}
	var vmStatus DeployVirtualMachineResponse
	err = i.do("deployVirtualMachine", paramsCopy, &vmStatus)
	if err != nil {
		return nil, err
	}
	rawWait, _ := i.base.GetConfigString("wait-timeout")
	maxWaitTime, _ := strconv.Atoi(rawWait)
	if maxWaitTime == 0 {
		maxWaitTime = 300
	}
	waitDuration := time.Duration(maxWaitTime) * time.Second
	job, err := q.EnqueueWait(i.taskName(machineCreateTaskName), monsterqueue.JobParams{
		"jobId":     vmStatus.DeployVirtualMachineResponse.JobID,
		"vmId":      vmStatus.DeployVirtualMachineResponse.ID,
		"projectId": params["projectid"],
	}, waitDuration)
	if err != nil {
		if err == monsterqueue.ErrQueueWaitTimeout {
			return nil, fmt.Errorf("cloudstack: time out after %v waiting for instance %s to start", waitDuration, vmStatus.DeployVirtualMachineResponse.ID)
		}
		return nil, err
	}
	result, err := job.Result()
	if err != nil {
		return nil, err
	}
	ipAddress := result.(string)
	m := &iaas.Machine{
		Id:      vmStatus.DeployVirtualMachineResponse.ID,
		Address: ipAddress,
		Status:  "running",
	}
	return m, nil
}

func (i *CloudstackIaaS) buildUrl(command string, params map[string]string) (string, error) {
	apiKey, err := i.base.GetConfigString("api-key")
	if err != nil {
		return "", err
	}
	secretKey, err := i.base.GetConfigString("secret-key")
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
	cloudstackUrl, err := i.base.GetConfigString("url")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?%s&signature=%s", cloudstackUrl, queryString, url.QueryEscape(signature)), nil
}

func (i *CloudstackIaaS) waitForAsyncJob(jobId string) (QueryAsyncJobResultResponse, error) {
	var jobResponse QueryAsyncJobResultResponse
	for {
		err := i.do("queryAsyncJobResult", ApiParams{"jobid": jobId}, &jobResponse)
		if err != nil {
			return jobResponse, err
		}
		if jobResponse.QueryAsyncJobResultResponse.JobStatus != jobInProgress {
			if jobResponse.QueryAsyncJobResultResponse.JobStatus == jobFailed {
				return jobResponse, fmt.Errorf("job failed to complete: %#v", jobResponse.QueryAsyncJobResultResponse.JobResult)
			}
			return jobResponse, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (i *CloudstackIaaS) waitVMIsCreated(jobId, machineId, projectId string) (string, error) {
	_, err := i.waitForAsyncJob(jobId)
	if err != nil {
		return "", err
	}
	var machineInfo ListVirtualMachinesResponse
	err = i.do("listVirtualMachines", ApiParams{
		"id":        machineId,
		"projectid": projectId,
	}, &machineInfo)
	if err != nil {
		return "", err
	}
	return machineInfo.ListVirtualMachinesResponse.VirtualMachine[0].Nic[0].IpAddress, nil
}
