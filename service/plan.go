// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"context"
)

// Plan represents a service plan
type Plan struct {
	Name        string
	Description string
}

func GetPlansByService(ctx context.Context, svc Service, pool, requestID string) ([]Plan, error) {
	endpoint, err := svc.getClientForPool(ctx, pool)
	if err != nil {
		return []Plan{}, nil
	}
	plans, err := endpoint.Plans(ctx, pool, requestID)
	if err != nil {
		return nil, err
	}
	return plans, nil
}

func GetPlanByServiceAndPlanName(ctx context.Context, svc Service, pool, planName, requestID string) (Plan, error) {
	plans, err := GetPlansByService(ctx, svc, pool, requestID)
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
