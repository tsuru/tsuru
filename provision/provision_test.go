// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"
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

func TestRegistry(t *testing.T) {
	var p1, p2 Provisioner
	Register("my-provisioner", p1)
	Register("your-provisioner", p2)
	provisioners := Registry()
	alt1 := []Provisioner{p1, p2}
	alt2 := []Provisioner{p2, p1}
	if !reflect.DeepEqual(provisioners, alt1) && !reflect.DeepEqual(provisioners, alt2) {
		t.Errorf("Registry(): Expected %#v. Got %#v.", alt1, provisioners)
	}
}

func TestError(t *testing.T) {
	errs := []*Error{
		{Reason: "something", Err: errors.New("went wrong")},
		{Reason: "something went wrong"},
	}
	expected := []string{"went wrong: something", "something went wrong"}
	for i := range errs {
		if errs[i].Error() != expected[i] {
			t.Errorf("Error.Error(): want %q. Got %q.", expected[i], errs[i].Error())
		}
	}
}

func TestErrorImplementsError(t *testing.T) {
	var _ error = &Error{}
}

func TestStatusString(t *testing.T) {
	var s Status = "pending"
	got := s.String()
	if got != "pending" {
		t.Errorf("Status.String(). want \"pending\". Got %q.", got)
	}
}
