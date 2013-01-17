// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package juju

import "github.com/globocom/tsuru/heal"

func init() {
	heal.Register("bootstrap", &BootstrapHealer{})
}

// BootstrapHealer is an implementation for the Healer interface. For more
// details on how a healer work, check the documentation of the heal package.
type BootstrapHealer struct{}

func (h *BootstrapHealer) Check() bool {
	return false
}

func (h *BootstrapHealer) Heal() error {
	return nil
}
