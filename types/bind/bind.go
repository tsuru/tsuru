// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package bind

// EnvVar represents a environment variable for an app.
type EnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Alias     string `json:"alias"`
	Public    bool   `json:"public"`
	ManagedBy string `json:"managedBy,omitempty"`
}

type ServiceEnvVar struct {
	EnvVar       `bson:",inline"`
	ServiceName  string `json:"-"`
	InstanceName string `json:"-"`
}
