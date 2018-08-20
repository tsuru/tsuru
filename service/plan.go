// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	osb "github.com/pmorie/go-open-service-broker-client/v2"
)

// Plan represents a service plan
type Plan struct {
	Name        string
	Description string
	Schemas     *osb.Schemas `json:",omitempty"`
}

func GetPlansByService(svc Service, requestID string) ([]Plan, error) {
	endpoint, err := svc.getClient("production")
	if err != nil {
		return []Plan{}, nil
	}
	plans, err := endpoint.Plans(requestID)
	if err != nil {
		return nil, err
	}
	return plans, nil
}

func GetPlanByServiceAndPlanName(svc Service, planName, requestID string) (Plan, error) {
	plans, err := GetPlansByService(svc, requestID)
	if err != nil {
		return Plan{}, err
	}
	for _, plan := range plans {
		if plan.Name == planName {
			return plan, nil
		}
	}
	return Plan{}, nil
}
