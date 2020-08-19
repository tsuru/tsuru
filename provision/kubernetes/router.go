// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	appTypes "github.com/tsuru/tsuru/types/app"
)

func (p *kubernetesProvisioner) EnsureRouter(app appTypes.App, routerType string, opts map[string]string) error {
	return nil
}
