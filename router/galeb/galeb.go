package galeb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tsuru/config"
)

var timeoutHttpClient = clientWithTimeout(10 * time.Second)

type commonResponse struct {
	Links struct {
		Self string `json:"self"`
	} `json:"_links"`
	Id     int    `json:"id"`
	Status string `json:"status"`
}

func (c commonResponse) FullId() string {
	return c.Links.Self
}

type backendPoolParams struct {
	Name              string `json:"name"`
	Environment       string `json:"environment"`
	FarmType          string `json:"farmtype"`
	Plan              string `json:"plan"`
	Project           string `json:"project"`
	LoadBalancePolicy string `json:"loadbalancepolicy"`
}

type backendParams struct {
	Ip          string `json:"ip"`
	Port        int    `json:"port"`
	BackendPool string `json:"backendpool"`
}

type ruleParams struct {
	Name        string `json:"name"`
	Match       string `json:"match"`
	BackendPool string `json:"backendpool"`
	RuleType    string `json:"ruletype"`
	Project     string `json:"project"`
}

type virtualHostParams struct {
	Name        string `json:"name"`
	FarmType    string `json:"farmtype"`
	Plan        string `json:"plan"`
	Environment string `json:"environment"`
	Project     string `json:"project"`
	RuleDefault string `json:"rule_default"`
}

type virtualHostRuleParams struct {
	Order       int    `json:"order"`
	VirtualHost string `json:"virtualhost"`
	Rule        string `json:"rule"`
}

type galebClient struct {
	apiUrl            string
	username          string
	password          string
	environment       string
	farmType          string
	plan              string
	project           string
	loadBalancePolicy string
	ruleType          string
}

func NewGalebClient() (*galebClient, error) {
	apiUrl, err := config.GetString("galeb:api-url")
	if err != nil {
		return nil, err
	}
	username, err := config.GetString("galeb:username")
	if err != nil {
		return nil, err
	}
	password, err := config.GetString("galeb:password")
	if err != nil {
		return nil, err
	}
	environment, _ := config.GetString("galeb:environment")
	farmType, _ := config.GetString("galeb:farm-type")
	plan, _ := config.GetString("galeb:plan")
	project, _ := config.GetString("galeb:project")
	loadBalancePolicy, _ := config.GetString("galeb:load-balance-policy")
	ruleType, _ := config.GetString("galeb:rule-type")
	return &galebClient{
		apiUrl:            apiUrl,
		username:          username,
		password:          password,
		environment:       environment,
		farmType:          farmType,
		plan:              plan,
		project:           project,
		loadBalancePolicy: loadBalancePolicy,
		ruleType:          ruleType,
	}, nil
}

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

func (c *galebClient) doRequest(method, path string, params interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	if params != nil {
		err := json.NewEncoder(&buf).Encode(params)
		if err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(c.apiUrl, "/"), strings.TrimLeft(path, "/"))
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	rsp, err := timeoutHttpClient.Do(req)
	return rsp, err
}

func (c *galebClient) doCreateResource(path string, params interface{}) (string, error) {
	rsp, err := c.doRequest("POST", path, params)
	if err != nil {
		return "", err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("POST %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	var commonRsp commonResponse
	err = json.Unmarshal(responseData, &commonRsp)
	if err != nil {
		return "", fmt.Errorf("POST %s: unable to parse response: %s: %s", path, string(responseData), err.Error())
	}
	return commonRsp.FullId(), nil
}

func (c *galebClient) fillDefaultBackendPoolValues(params *backendPoolParams) {
	if params.Environment == "" {
		params.Environment = c.environment
	}
	if params.LoadBalancePolicy == "" {
		params.LoadBalancePolicy = c.loadBalancePolicy
	}
	if params.Plan == "" {
		params.Plan = c.plan
	}
	if params.Project == "" {
		params.Project = c.project
	}
	if params.FarmType == "" {
		params.FarmType = c.farmType
	}
}

func (c *galebClient) fillDefaultRuleValues(params *ruleParams) {
	if params.RuleType == "" {
		params.RuleType = c.ruleType
	}
	if params.Project == "" {
		params.Project = c.project
	}
}

func (c *galebClient) fillDefaultVirtualHostValues(params *virtualHostParams) {
	if params.Environment == "" {
		params.Environment = c.environment
	}
	if params.FarmType == "" {
		params.FarmType = c.farmType
	}
	if params.Plan == "" {
		params.Plan = c.plan
	}
	if params.Project == "" {
		params.Project = c.project
	}
}

func (c *galebClient) AddBackendPool(params *backendPoolParams) (string, error) {
	c.fillDefaultBackendPoolValues(params)
	return c.doCreateResource("/backendpool/", params)
}

func (c *galebClient) AddBackend(params *backendParams) (string, error) {
	return c.doCreateResource("/backend/", params)
}

func (c *galebClient) AddRule(params *ruleParams) (string, error) {
	c.fillDefaultRuleValues(params)
	return c.doCreateResource("/rule/", params)
}

func (c *galebClient) AddVirtualHost(params *virtualHostParams) (string, error) {
	c.fillDefaultVirtualHostValues(params)
	return c.doCreateResource("/virtualhost/", params)
}

func (c *galebClient) AddVirtualHostRule(params *virtualHostRuleParams) (string, error) {
	return c.doCreateResource("/virtualhostrule/", params)
}

func (c *galebClient) RemoveResource(resourceURI string) error {
	path := strings.TrimPrefix(resourceURI, c.apiUrl)
	rsp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	responseData, _ := ioutil.ReadAll(rsp.Body)
	if rsp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("DELETE %s: invalid response code: %d: %s", path, rsp.StatusCode, string(responseData))
	}
	return nil
}
