// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

// Plan represents a service plan
type Plan struct {
	Name        string
	Description string
}

func GetPlansByServiceName(serviceName, requestID string) ([]Plan, error) {
	s, err := Get(serviceName)
	if err != nil {
		return nil, err
	}
	endpoint, err := s.getClient("production")
	if err != nil {
		return []Plan{}, nil
	}
	plans, err := endpoint.Plans(requestID)
	if err != nil {
		return nil, err
	}
	return plans, nil
}

func GetPlanByServiceNameAndPlanName(serviceName, planName, requestID string) (Plan, error) {
	plans, err := GetPlansByServiceName(serviceName, requestID)
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
