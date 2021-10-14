//go:build tools
// +build tools

// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

// This is used by /hack/update-codegen.sh
import (
	_ "k8s.io/code-generator"
)
