// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"net/url"

	"github.com/cezarsa/form"
	"github.com/pkg/errors"
)

type Payload interface {
	// Form is used to convert to the legacy well known format
	Form() url.Values
}

var (
	_ Payload = &createServicePayload{}
	_ Payload = &updateServicePayload{}
	_ Payload = &destroyServicePayload{}
	_ Payload = &bindAppPayload{}
	_ Payload = &unbindAppPayload{}
	_ Payload = &bindJobPayload{}
)

type createServicePayload struct {
	Name        string         `json:"name"`
	Team        string         `json:"team"`
	Description string         `json:"description,omitempty"`
	Plan        string         `json:"plan,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	EventID     string         `json:"eventID"`
	User        string         `json:"user"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

func (r *createServicePayload) Form() url.Values {
	params := url.Values{
		"name":    {r.Name},
		"team":    {r.Team},
		"user":    {r.User},
		"eventid": {r.EventID},
		"tags":    r.Tags,
	}

	if r.Plan != "" {
		params["plan"] = []string{r.Plan}
	}
	if r.Description != "" {
		params["description"] = []string{r.Description}
	}
	addParameters(params, r.Parameters)

	return params
}

type updateServicePayload struct {
	Description string         `json:"description,omitempty"`
	Team        string         `json:"team"`
	Plan        string         `json:"plan,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	EventID     string         `json:"eventID"`
	User        string         `json:"user"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

func (r *updateServicePayload) Form() url.Values {
	params := url.Values{
		"description": {r.Description},
		"team":        {r.Team},
		"tags":        r.Tags,
		"plan":        {r.Plan},
		"user":        {r.User},
		"eventid":     {r.EventID},
	}

	addParameters(params, r.Parameters)

	return params
}

type destroyServicePayload struct {
	EventID string `json:"eventID"`
	User    string `json:"user"`
}

func (r *destroyServicePayload) Form() url.Values {
	return url.Values{
		"user":    {r.User},
		"eventid": {r.EventID},
	}
}

type unbindAppPayload struct {
	AppHosts []string `json:"appHosts"`
	AppName  string   `json:"appName"`
	User     string   `json:"user"`
	EventID  string   `json:"eventID"`
}

func (r *unbindAppPayload) Form() url.Values {
	params := url.Values{
		"app-hosts": r.AppHosts,
		"app-name":  {r.AppName},
		"user":      {r.User},
		"eventid":   {r.EventID},
	}

	if len(r.AppHosts) > 0 {
		params["app-host"] = []string{r.AppHosts[0]}
	}

	return params
}

func addParameters(dst url.Values, params map[string]interface{}) {
	if params == nil || dst == nil {
		return
	}
	encoded, err := form.EncodeToValues(params)
	if err != nil {
		errors.Wrapf(err, "unable to encode parameters")
	}
	for key, value := range encoded {
		dst["parameters."+key] = value
	}
}

type bindAppPayload struct {
	AppName               string         `json:"appName"`
	Parameters            map[string]any `json:"parameters,omitempty"`
	User                  string         `json:"user,omitempty"`
	EventID               string         `json:"eventID,omitempty"`
	AppHosts              []string       `json:"appHosts,omitempty"`
	AppInternalHosts      []string       `json:"appInternalHosts,omitempty"`
	AppPoolName           string         `json:"appPoolName,omitempty"`
	AppPoolProvisioner    string         `json:"appPoolProvisioner,omitempty"`
	AppClusterName        string         `json:"appClusterName,omitempty"`
	AppClusterProvisioner string         `json:"appClusterProvisioner,omitempty"`
	AppClusterAddresses   []string       `json:"appClusterAddresses,omitempty"`
}

func (r *bindAppPayload) Form() url.Values {
	params := url.Values{
		"app-name": {r.AppName},
	}
	addParameters(params, r.Parameters)

	if r.User != "" {
		params.Set("user", r.User)
	}
	if r.EventID != "" {
		params.Set("eventid", r.EventID)
	}

	params["app-hosts"] = r.AppHosts
	if len(r.AppHosts) > 0 {
		params.Set("app-host", r.AppHosts[0])
	}

	params["app-internal-hosts"] = r.AppInternalHosts

	if r.AppPoolName != "" {
		params.Set("app-pool-name", r.AppPoolName)
	}

	if r.AppPoolProvisioner != "" {
		params.Set("app-pool-provisioner", r.AppPoolProvisioner)
	}
	if r.AppClusterName != "" {

		params.Set("app-cluster-name", r.AppClusterName)
	}

	if r.AppClusterProvisioner != "" {
		params.Set("app-cluster-provisioner", r.AppClusterProvisioner)
	}
	for _, addr := range r.AppClusterAddresses {
		params.Add("app-cluster-addresses", addr)
	}
	return params
}

type bindJobPayload struct {
	JobName string `json:"jobName"`
	User    string `json:"user,omitempty"`
	EventID string `json:"eventID,omitempty"`

	JobPoolName    string `json:"jobPoolName,omitempty"`
	JobClusterName string `json:"jobClusterName,omitempty"`
}

func (r *bindJobPayload) Form() url.Values {
	params := url.Values{
		"job-name": {r.JobName},
	}

	if r.User != "" {
		params.Set("user", r.User)
	}

	if r.EventID != "" {
		params.Set("eventid", r.EventID)
	}

	if r.JobPoolName != "" {
		params.Set("job-pool-name", r.JobPoolName)
	}
	if r.JobClusterName != "" {
		params.Set("job-cluster-name", r.JobClusterName)
	}
	return params
}
