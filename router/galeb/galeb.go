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
	"github.com/tsuru/tsuru/errors"
)

var timeoutHttpClient = clientWithTimeout(10 * time.Second)

type backendPoolParams struct {
	Name              string `json:"name"`
	Environment       string `json:"environment"`
	Plan              string `json:"plan"`
	Project           string `json:"project"`
	LoadBalancePolicy string `json:"loadbalancepolicy"`
}

// curl -H "Content-Type: application/json" -X POST http://user:123@galeb.somewhere.com/api/backendpool/ -d '
// {
//   "name": "curl test",
//   "environment": "http://galeb.somewhere.com/api/environment/1/",
//   "farmtype": "http://galeb.somewhere.com/api/farmtype/1/",
//   "plan": "http://galeb.somewhere.com/api/plan/1/",
//   "project": "http://galeb.somewhere.com/api/project/1/",
//   "loadbalancepolicy": "http://galeb.somewhere.com/api/loadbalancepolicy/1/"
// }'

type backendParams struct {
	Ip          string `json:"ip"`
	Port        int    `json:"port"`
	BackendPool string `json:"backendpool"`
}

// curl -H "Content-Type: application/json" -X POST http://user:123@galeb.somewhere.com/api/backend/ -d '
// {
//   "ip": "10.10.10.10",
//   "port": 80,
//   "backendpool": "http://galeb.somewhere.com/api/backendpool/1/"
// }'

type ruleParams struct {
	Name        string `json:"name"`
	Match       string `json:"match"`
	BackendPool string `json:"backendpool"`
	RuleType    string `json:"ruletype"`
	Project     string `json:"project"`
}

// curl -H "Content-Type: application/json" -X POST http://user:123@galeb.somewhere.com/api/rule/ -d '
// {
//    "name": "root rule",
//    "match": "/",
//    "backendpool": "http://galeb.somewhere.com/api/backendpool/1/",
//    "ruletype": "http://galeb.somewhere.com/api/ruletype/1/",
//    "project": "http://galeb.somewhere.com/api/project/1/"
// }'

type virtualHostParams struct {
	Name        string `json:"name"`
	FarmType    string `json:"farmtype"`
	Plan        string `json:"plan"`
	Environment string `json:"environment"`
	Project     string `json:"project"`
	RuleDefault string `json:"rule_default"`
}

// curl -H "Content-Type: application/json" -X POST http://user:123@galeb.somewhere.com/api/virtualhost/ -d '
// {
//    "name": "test.virtualhost.api",
//    "farmtype": "http://galeb.somewhere.com/api/farmtype/1/",
//    "plan": "http://galeb.somewhere.com/api/plan/1/",
//    "environment": "http://galeb.somewhere.com/api/environment/1/",
//    "project": "http://galeb.somewhere.com/api/project/1/",
//    "rule_default": "http://galeb.somewhere.com/api/rule/1/"
// }'

type virtualHostRuleParams struct {
	Order       int    `json:"order"`
	VirtualHost string `json:"virtualhost"`
	Rule        string `json:"rule"`
}

// curl -H "Content-Type: application/json" -X POST http://user:123@galeb.somewhere.com/api/virtualhostrule/ -d '
// {
//    "order": 1,
//    "virtualhost": "http://galeb.somewhere.com/api/virtualhost/1/",
//    "rule": "http://galeb.somewhere.com/api/rule/1/"
// }'

type galebClient struct {
	apiUrl   string
	username string
	password string
}

func newGalebClient() (*galebClient, error) {
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
	return &galebClient{
		apiUrl:   apiUrl,
		username: username,
		password: password,
	}, nil
}

func (c *galebClient) doRequest(method, path string, body interface{}) (*http.Response, error) {
	buf := bytes.Buffer{}
	err := json.NewEncoder(&buf).Encode(body)
	if err != nil {
		return nil, err
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

func (c *galebClient) addBackendPool(params *backendPoolParams) error {
	rsp, err := c.doRequest("POST", "/api/backendpool/", params)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusCreated {
		responseData, _ := ioutil.ReadAll(rsp.Body)
		return &errors.HTTP{Code: rsp.StatusCode, Message: string(responseData)}
	}
	return nil
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
