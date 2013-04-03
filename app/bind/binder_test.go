// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"testing"
)

func TestEnvVarStringPrintPublicValue(t *testing.T) {
	env := EnvVar{Name: "PATH", Value: "/", Public: true}
	if env.String() != "/" {
		t.Errorf("Should print public variable value.\nExpected: /\nGot: %s", env.String())
	}
}

func TestEnvVarStringMaskPrivateValue(t *testing.T) {
	env := EnvVar{Name: "PATH", Value: "/", Public: false}
	if env.String() != "*** (private variable)" {
		t.Errorf("Should omit private variable value.\nExpected: *** (private variable)\nGot: %s", env.String())
	}
}
