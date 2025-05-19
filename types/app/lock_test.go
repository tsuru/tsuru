// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"strings"
	"testing"
	"time"
)

func TestAppLockString(t *testing.T) {

	acquireDate := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	lock := &AppLock{Locked: true, Reason: "maintenance", Owner: "admin", AcquireDate: acquireDate}
	result := lock.String()

	if !strings.Contains(result, "admin") {
		t.Errorf("Expected owner 'admin' in output, got: %s", result)
	}
	if !strings.Contains(result, "maintenance") {
		t.Errorf("Expected reason 'maintenance' in output, got: %s", result)
	}
	if !strings.Contains(result, acquireDate.String()) {
		t.Errorf("Expected acquire date '%s' in output, got: %s", acquireDate.String(), result)
	}
}

func TestAppLockImplementsInterface(t *testing.T) {
	var _ AppLockInterface = &AppLock{}
}
