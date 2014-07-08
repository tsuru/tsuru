// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/iaas"
	_ "github.com/tsuru/tsuru/iaas/cloudstack"
)

func GetIaaS() (iaas.IaaS, error) {
	providerName, err := config.GetString("iaas:provider")
	if err != nil {
		return nil, err
	}
	return iaas.GetIaasProvider(providerName)
}
