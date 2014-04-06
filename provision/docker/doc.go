// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package docker provides a provisioner implementation that use Docker
// containers.
//
// In order to use the provisioner, just import tsuru's provision package and
// docker provision package. Then call provision.Get("docker") to get an
// instance of Docker provisioner:
//
//     import (
//         "github.com/tsuru/tsuru/provision"
//         _ "github.com/tsuru/tsuru/provision/docker"
//     )
//     // ...
//     func main() {
//         provisioner, err := provision.Get("docker")
//         // Use provisioner.
//     }
package docker
