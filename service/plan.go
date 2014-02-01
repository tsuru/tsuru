// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import "errors"

// Plan represents a service plan
type Plan struct {
	Name        string
	Description string
}

func GetPlansByServiceName(serviceName string) ([]Plan, error) {
	s := Service{Name: serviceName}
	err := s.Get()
	if err != nil {
		return nil, err
	}
	endpoint, err := s.getClient("production")
	if err != nil {
		return nil, errors.New("endpoint does not exists")
	}
	plans, err := endpoint.Plans()
	if err != nil {
		return nil, err
	}
	return plans, nil
}
