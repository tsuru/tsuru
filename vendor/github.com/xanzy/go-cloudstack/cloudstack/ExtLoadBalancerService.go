//
// Copyright 2018, Sander van Harmelen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package cloudstack

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type AddExternalLoadBalancerParams struct {
	p map[string]interface{}
}

func (p *AddExternalLoadBalancerParams) toURLValues() url.Values {
	u := url.Values{}
	if p.p == nil {
		return u
	}
	if v, found := p.p["password"]; found {
		u.Set("password", v.(string))
	}
	if v, found := p.p["url"]; found {
		u.Set("url", v.(string))
	}
	if v, found := p.p["username"]; found {
		u.Set("username", v.(string))
	}
	if v, found := p.p["zoneid"]; found {
		u.Set("zoneid", v.(string))
	}
	return u
}

func (p *AddExternalLoadBalancerParams) SetPassword(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["password"] = v
	return
}

func (p *AddExternalLoadBalancerParams) SetUrl(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["url"] = v
	return
}

func (p *AddExternalLoadBalancerParams) SetUsername(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["username"] = v
	return
}

func (p *AddExternalLoadBalancerParams) SetZoneid(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["zoneid"] = v
	return
}

// You should always use this function to get a new AddExternalLoadBalancerParams instance,
// as then you are sure you have configured all required params
func (s *ExtLoadBalancerService) NewAddExternalLoadBalancerParams(password string, url string, username string, zoneid string) *AddExternalLoadBalancerParams {
	p := &AddExternalLoadBalancerParams{}
	p.p = make(map[string]interface{})
	p.p["password"] = password
	p.p["url"] = url
	p.p["username"] = username
	p.p["zoneid"] = zoneid
	return p
}

// Adds F5 external load balancer appliance.
func (s *ExtLoadBalancerService) AddExternalLoadBalancer(p *AddExternalLoadBalancerParams) (*AddExternalLoadBalancerResponse, error) {
	resp, err := s.cs.newRequest("addExternalLoadBalancer", p.toURLValues())
	if err != nil {
		return nil, err
	}

	var r AddExternalLoadBalancerResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

type AddExternalLoadBalancerResponse struct {
	Id               string `json:"id"`
	Ipaddress        string `json:"ipaddress"`
	Numretries       string `json:"numretries"`
	Privateinterface string `json:"privateinterface"`
	Publicinterface  string `json:"publicinterface"`
	Username         string `json:"username"`
	Zoneid           string `json:"zoneid"`
}

type DeleteExternalLoadBalancerParams struct {
	p map[string]interface{}
}

func (p *DeleteExternalLoadBalancerParams) toURLValues() url.Values {
	u := url.Values{}
	if p.p == nil {
		return u
	}
	if v, found := p.p["id"]; found {
		u.Set("id", v.(string))
	}
	return u
}

func (p *DeleteExternalLoadBalancerParams) SetId(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["id"] = v
	return
}

// You should always use this function to get a new DeleteExternalLoadBalancerParams instance,
// as then you are sure you have configured all required params
func (s *ExtLoadBalancerService) NewDeleteExternalLoadBalancerParams(id string) *DeleteExternalLoadBalancerParams {
	p := &DeleteExternalLoadBalancerParams{}
	p.p = make(map[string]interface{})
	p.p["id"] = id
	return p
}

// Deletes a F5 external load balancer appliance added in a zone.
func (s *ExtLoadBalancerService) DeleteExternalLoadBalancer(p *DeleteExternalLoadBalancerParams) (*DeleteExternalLoadBalancerResponse, error) {
	resp, err := s.cs.newRequest("deleteExternalLoadBalancer", p.toURLValues())
	if err != nil {
		return nil, err
	}

	var r DeleteExternalLoadBalancerResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

type DeleteExternalLoadBalancerResponse struct {
	Displaytext string `json:"displaytext"`
	Success     bool   `json:"success"`
}

func (r *DeleteExternalLoadBalancerResponse) UnmarshalJSON(b []byte) error {
	var m map[string]interface{}
	err := json.Unmarshal(b, &m)
	if err != nil {
		return err
	}

	if success, ok := m["success"].(string); ok {
		m["success"] = success == "true"
		b, err = json.Marshal(m)
		if err != nil {
			return err
		}
	}

	type alias DeleteExternalLoadBalancerResponse
	return json.Unmarshal(b, (*alias)(r))
}

type ListExternalLoadBalancersParams struct {
	p map[string]interface{}
}

func (p *ListExternalLoadBalancersParams) toURLValues() url.Values {
	u := url.Values{}
	if p.p == nil {
		return u
	}
	if v, found := p.p["keyword"]; found {
		u.Set("keyword", v.(string))
	}
	if v, found := p.p["page"]; found {
		vv := strconv.Itoa(v.(int))
		u.Set("page", vv)
	}
	if v, found := p.p["pagesize"]; found {
		vv := strconv.Itoa(v.(int))
		u.Set("pagesize", vv)
	}
	if v, found := p.p["zoneid"]; found {
		u.Set("zoneid", v.(string))
	}
	return u
}

func (p *ListExternalLoadBalancersParams) SetKeyword(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["keyword"] = v
	return
}

func (p *ListExternalLoadBalancersParams) SetPage(v int) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["page"] = v
	return
}

func (p *ListExternalLoadBalancersParams) SetPagesize(v int) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["pagesize"] = v
	return
}

func (p *ListExternalLoadBalancersParams) SetZoneid(v string) {
	if p.p == nil {
		p.p = make(map[string]interface{})
	}
	p.p["zoneid"] = v
	return
}

// You should always use this function to get a new ListExternalLoadBalancersParams instance,
// as then you are sure you have configured all required params
func (s *ExtLoadBalancerService) NewListExternalLoadBalancersParams() *ListExternalLoadBalancersParams {
	p := &ListExternalLoadBalancersParams{}
	p.p = make(map[string]interface{})
	return p
}

// This is a courtesy helper function, which in some cases may not work as expected!
func (s *ExtLoadBalancerService) GetExternalLoadBalancerID(keyword string, opts ...OptionFunc) (string, int, error) {
	p := &ListExternalLoadBalancersParams{}
	p.p = make(map[string]interface{})

	p.p["keyword"] = keyword

	for _, fn := range append(s.cs.options, opts...) {
		if err := fn(s.cs, p); err != nil {
			return "", -1, err
		}
	}

	l, err := s.ListExternalLoadBalancers(p)
	if err != nil {
		return "", -1, err
	}

	if l.Count == 0 {
		return "", l.Count, fmt.Errorf("No match found for %s: %+v", keyword, l)
	}

	if l.Count == 1 {
		return l.ExternalLoadBalancers[0].Id, l.Count, nil
	}

	if l.Count > 1 {
		for _, v := range l.ExternalLoadBalancers {
			if v.Name == keyword {
				return v.Id, l.Count, nil
			}
		}
	}
	return "", l.Count, fmt.Errorf("Could not find an exact match for %s: %+v", keyword, l)
}

// Lists F5 external load balancer appliances added in a zone.
func (s *ExtLoadBalancerService) ListExternalLoadBalancers(p *ListExternalLoadBalancersParams) (*ListExternalLoadBalancersResponse, error) {
	resp, err := s.cs.newRequest("listExternalLoadBalancers", p.toURLValues())
	if err != nil {
		return nil, err
	}

	var r ListExternalLoadBalancersResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

type ListExternalLoadBalancersResponse struct {
	Count                 int                     `json:"count"`
	ExternalLoadBalancers []*ExternalLoadBalancer `json:"externalloadbalancer"`
}

type ExternalLoadBalancer struct {
	Averageload             int64             `json:"averageload"`
	Capabilities            string            `json:"capabilities"`
	Clusterid               string            `json:"clusterid"`
	Clustername             string            `json:"clustername"`
	Clustertype             string            `json:"clustertype"`
	Cpuallocated            string            `json:"cpuallocated"`
	Cpunumber               int               `json:"cpunumber"`
	Cpusockets              int               `json:"cpusockets"`
	Cpuspeed                int64             `json:"cpuspeed"`
	Cpuused                 string            `json:"cpuused"`
	Cpuwithoverprovisioning string            `json:"cpuwithoverprovisioning"`
	Created                 string            `json:"created"`
	Details                 map[string]string `json:"details"`
	Disconnected            string            `json:"disconnected"`
	Disksizeallocated       int64             `json:"disksizeallocated"`
	Disksizetotal           int64             `json:"disksizetotal"`
	Events                  string            `json:"events"`
	Gpugroup                []struct {
		Gpugroupname string `json:"gpugroupname"`
		Vgpu         []struct {
			Maxcapacity       int64  `json:"maxcapacity"`
			Maxheads          int64  `json:"maxheads"`
			Maxresolutionx    int64  `json:"maxresolutionx"`
			Maxresolutiony    int64  `json:"maxresolutiony"`
			Maxvgpuperpgpu    int64  `json:"maxvgpuperpgpu"`
			Remainingcapacity int64  `json:"remainingcapacity"`
			Vgputype          string `json:"vgputype"`
			Videoram          int64  `json:"videoram"`
		} `json:"vgpu"`
	} `json:"gpugroup"`
	Hahost               bool                        `json:"hahost"`
	Hasenoughcapacity    bool                        `json:"hasenoughcapacity"`
	Hosttags             string                      `json:"hosttags"`
	Hypervisor           string                      `json:"hypervisor"`
	Hypervisorversion    string                      `json:"hypervisorversion"`
	Id                   string                      `json:"id"`
	Ipaddress            string                      `json:"ipaddress"`
	Islocalstorageactive bool                        `json:"islocalstorageactive"`
	Lastpinged           string                      `json:"lastpinged"`
	Managementserverid   int64                       `json:"managementserverid"`
	Memoryallocated      int64                       `json:"memoryallocated"`
	Memorytotal          int64                       `json:"memorytotal"`
	Memoryused           int64                       `json:"memoryused"`
	Name                 string                      `json:"name"`
	Networkkbsread       int64                       `json:"networkkbsread"`
	Networkkbswrite      int64                       `json:"networkkbswrite"`
	Oscategoryid         string                      `json:"oscategoryid"`
	Oscategoryname       string                      `json:"oscategoryname"`
	Outofbandmanagement  OutOfBandManagementResponse `json:"outofbandmanagement"`
	Podid                string                      `json:"podid"`
	Podname              string                      `json:"podname"`
	Removed              string                      `json:"removed"`
	Resourcestate        string                      `json:"resourcestate"`
	State                string                      `json:"state"`
	Suitableformigration bool                        `json:"suitableformigration"`
	Type                 string                      `json:"type"`
	Version              string                      `json:"version"`
	Zoneid               string                      `json:"zoneid"`
	Zonename             string                      `json:"zonename"`
}
