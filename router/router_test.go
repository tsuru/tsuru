// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"reflect"
	"testing"
)

func TestRegisterAndGet(t *testing.T) {
	var r Router
	Register("router", r)
	got, err := Get("router")
	if err != nil {
		t.Fatalf("Got unexpected error when getting router: %q", err)
	}
	if !reflect.DeepEqual(r, got) {
		t.Errorf("Get: Want %#v. Got %#v.", r, got)
	}
	_, err = Get("unknown-router")
	if err == nil {
		t.Errorf("Expected non-nil error when getting unknown router, got <nil>.")
	}
	expectedMessage := `Unknown router: "unknown-router".`
	if err.Error() != expectedMessage {
		t.Errorf("Expected error %q. Got %q.", expectedMessage, err.Error())
	}
}
