// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iaas

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/hc"
)

// BuildHealthCheck creates a healthcheck functions for the given providerName.
//
// It will call the HealthCheck() method in the provider (only if it's also a
// HealthChecker), for each instance of it (including the "main" instance and
// all custom IaaSes).
func BuildHealthCheck(providerName string) func() error {
	return func() error {
		iaasConfig, err := config.Get("iaas")
		if err != nil {
			return hc.ErrDisabledComponent
		}
		iaases, _ := iaasConfig.(map[interface{}]interface{})
		for ifaceName := range iaases {
			name := ifaceName.(string)
			if name == "custom" {
				customIaases := iaases[name].(map[interface{}]interface{})
				for ifaceName := range customIaases {
					iaas := customIaases[ifaceName.(string)].(map[interface{}]interface{})
					if iaas["provider"].(string) != providerName {
						continue
					}
					name = ifaceName.(string)
				}
			} else if name != providerName {
				continue
			}
			err := healthCheck(name)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func healthCheck(name string) error {
	provider, err := GetIaasProvider(name)
	if err != nil {
		return err
	}
	if hprovider, ok := provider.(HealthChecker); ok {
		return hprovider.HealthCheck()
	}
	return hc.ErrDisabledComponent
}
