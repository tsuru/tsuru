// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package juju provide utilities functions for interaction with Juju. It also
// provides a provisioner implementation for Juju.
//
// In order to use the provisioner, just import tsuru's provision package and
// juju provision package. Then call provision.Get("juju") to get an instance
// of JujuProvisioner:
//
//     import (
//         "github.com/tsuru/tsuru/provision"
//         _ "github.com/tsuru/tsuru/provision/juju"
//     )
//     // ...
//     func main() {
//         provisioner, err := provision.Get("juju")
//         // Use provisioner.
//     }
package juju
