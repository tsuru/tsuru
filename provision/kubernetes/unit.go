// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"

	appTypes "github.com/tsuru/tsuru/types/app"
)

func (p *kubernetesProvisioner) KillUnit(ctx context.Context, app appTypes.App, unitName string, force bool) error {
	return nil
}
