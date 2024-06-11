// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package envs

import (
	"sort"
	"testing"

	bindTypes "github.com/tsuru/tsuru/types/bind"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestEnvsInterpolate(c *check.C) {
	mergedEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"MyVariable":     {Name: "MyVariable", Alias: "MY_VAR"},
		"MyService":      {Name: "MyService", Alias: "MY_SERVICE_VAR"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	toInterpolate := map[string]string{
		"MyVariable": "MY_VAR",
		"MyService":  "MY_SERVICE_VAR",
	}
	//ordered
	toInterpolateKeys := []string{
		"MyService",
		"MyVariable",
	}
	sort.Strings(toInterpolateKeys)

	for _, envName := range toInterpolateKeys {
		Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	expectEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"MyVariable":     {Name: "MyVariable", Value: "my-value", Alias: "MY_VAR"},
		"MyService":      {Name: "MyService", Value: "my-service-value", Alias: "MY_SERVICE_VAR"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	c.Assert(mergedEnvs, check.DeepEquals, expectEnvs)
}

func (s *S) TestEnvsInterpolateSelfReference(c *check.C) {
	mergedEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"self":           {Name: "self", Value: "self-value", Alias: "self"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	toInterpolate := map[string]string{
		"self": "self",
	}
	//ordered
	toInterpolateKeys := []string{
		"self",
	}

	for _, envName := range toInterpolateKeys {
		Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	expectEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"self":           {Name: "self", Value: "self-value", Alias: "self"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	c.Assert(mergedEnvs, check.DeepEquals, expectEnvs)
}

func (s *S) TestEnvsInterpolateCircularReference(c *check.C) {
	mergedEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"from":           {Name: "from", Value: "A", Alias: "to"},
		"to":             {Name: "to", Value: "B", Alias: "then"},
		"then":           {Name: "then", Value: "C", Alias: "from"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	toInterpolate := map[string]string{
		"from": "to",
		"to":   "then",
		"then": "from",
	}
	//ordered
	toInterpolateKeys := []string{
		"from",
		"then",
		"to",
	}
	sort.Strings(toInterpolateKeys)

	for _, envName := range toInterpolateKeys {
		Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	expectEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"from":           {Name: "from", Value: "A", Alias: "to"},
		"to":             {Name: "to", Value: "A", Alias: "then"},
		"then":           {Name: "then", Value: "A", Alias: "from"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	c.Assert(mergedEnvs, check.DeepEquals, expectEnvs)
}

func (s *S) TestEnvsInterpolateInvalidAlias(c *check.C) {
	mergedEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"invalid":        {Name: "inv", Value: "", Alias: "notfound"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	toInterpolate := map[string]string{
		"invalid": "notfound",
	}
	//ordered
	toInterpolateKeys := []string{
		"invalid",
	}

	for _, envName := range toInterpolateKeys {
		Interpolate(mergedEnvs, toInterpolate, envName, toInterpolate[envName])
	}

	expectEnvs := map[string]bindTypes.EnvVar{
		"MY_VAR":         {Name: "MY_VAR", Value: "my-value"},
		"YOUR_VAR":       {Name: "YOUR_VAR", Value: "your-value"},
		"invalid":        {Name: "inv", Value: "", Alias: "notfound"},
		"MY_SERVICE_VAR": {Name: "MY_SERVICE_VAR", Value: "my-service-value", Public: true},
	}
	c.Assert(mergedEnvs, check.DeepEquals, expectEnvs)
}

func (s *S) TestServiceEnvsFromEnvVars(c *check.C) {
	serviceEnvs := []bindTypes.ServiceEnvVar{{
		EnvVar: bindTypes.EnvVar{
			Name:   "DB_HOST",
			Value:  "fake.host1",
			Public: true,
		},
		ServiceName:  "my-service",
		InstanceName: "my-instance-1",
	}}

	tsuruServicesEnvVar := ServiceEnvsFromEnvVars(serviceEnvs)

	expectedValue := "{\"my-service\":[{\"instance_name\":\"my-instance-1\",\"envs\":{\"DB_HOST\":\"fake.host1\"}}]}"
	expectedTsuruServicesEnvVar := bindTypes.EnvVar{
		Name:      TsuruServicesEnvVar,
		Value:     expectedValue,
		Public:    false,
		ManagedBy: "tsuru",
	}
	c.Assert(tsuruServicesEnvVar, check.DeepEquals, expectedTsuruServicesEnvVar)
}

func (s *S) TestServiceEnvsFromEnvVarsWithMultipleServiceInstances(c *check.C) {
	serviceEnvs := []bindTypes.ServiceEnvVar{{
		EnvVar: bindTypes.EnvVar{
			Name:   "DB_HOST",
			Value:  "fake.host1",
			Public: true,
		},
		ServiceName:  "my-service",
		InstanceName: "my-instance-1",
	}, {
		EnvVar: bindTypes.EnvVar{
			Name:   "DB_HOST",
			Value:  "fake.host2",
			Public: true,
		},
		ServiceName:  "my-service",
		InstanceName: "my-instance-2",
	}}

	tsuruServicesEnvVar := ServiceEnvsFromEnvVars(serviceEnvs)

	expectedValue := "{\"my-service\":[{\"instance_name\":\"my-instance-1\",\"envs\":{\"DB_HOST\":\"fake.host1\"}},{\"instance_name\":\"my-instance-2\",\"envs\":{\"DB_HOST\":\"fake.host2\"}}]}"
	expectedTsuruServicesEnvVar := bindTypes.EnvVar{
		Name:      TsuruServicesEnvVar,
		Value:     expectedValue,
		Public:    false,
		ManagedBy: "tsuru",
	}
	c.Assert(tsuruServicesEnvVar, check.DeepEquals, expectedTsuruServicesEnvVar)
}

func (s *S) TestServiceEnvsFromEnvVarsWithMultipleServices(c *check.C) {
	serviceEnvs := []bindTypes.ServiceEnvVar{{
		EnvVar: bindTypes.EnvVar{
			Name:   "DB_HOST",
			Value:  "fake.host1",
			Public: true,
		},
		ServiceName:  "my-service-1",
		InstanceName: "my-instance",
	}, {
		EnvVar: bindTypes.EnvVar{
			Name:   "DB_HOST",
			Value:  "fake.host2",
			Public: true,
		},
		ServiceName:  "my-service-2",
		InstanceName: "my-instance",
	}}

	tsuruServicesEnvVar := ServiceEnvsFromEnvVars(serviceEnvs)

	expectedValue := "{\"my-service-1\":[{\"instance_name\":\"my-instance\",\"envs\":{\"DB_HOST\":\"fake.host1\"}}],\"my-service-2\":[{\"instance_name\":\"my-instance\",\"envs\":{\"DB_HOST\":\"fake.host2\"}}]}"
	expectedTsuruServicesEnvVar := bindTypes.EnvVar{
		Name:      TsuruServicesEnvVar,
		Value:     expectedValue,
		Public:    false,
		ManagedBy: "tsuru",
	}
	c.Assert(tsuruServicesEnvVar, check.DeepEquals, expectedTsuruServicesEnvVar)
}
