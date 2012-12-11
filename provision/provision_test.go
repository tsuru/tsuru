// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"reflect"
	"testing"
)

func TestRegisterAndGetProvisioner(t *testing.T) {
	var p Provisioner
	Register("my-provisioner", p)
	got, err := Get("my-provisioner")
	if err != nil {
		t.Fatalf("Got unexpected error when getting provisioner: %q", err)
	}
	if !reflect.DeepEqual(p, got) {
		t.Errorf("Get: Want %#v. Got %#v.", p, got)
	}
	_, err = Get("unknown-provisioner")
	if err == nil {
		t.Errorf("Expected non-nil error when getting unknown provisioner, got <nil>.")
	}
	expectedMessage := `Unknown provisioner: "unknown-provisioner".`
	if err.Error() != expectedMessage {
		t.Errorf("Expected error %q. Got %q.", expectedMessage, err.Error())
	}
}

func TestStatuses(t *testing.T) {
	names := []Status{Started, Pending, Down, Error}
	values := []string{"started", "pending", "down", "error"}
	for i := range names {
		if string(names[i]) != values[i] {
			t.Errorf("Status: Want %q. Got %q.", values[i], names[i])
		}
	}
}
