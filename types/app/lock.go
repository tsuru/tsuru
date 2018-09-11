// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"time"
)

// AppLock stores information about a lock hold on the app
type AppLock struct {
	Locked      bool
	Reason      string
	Owner       string
	AcquireDate time.Time
}

type ErrAppNotLocked struct {
	App string
}

func (l *AppLock) String() string {
	if !l.Locked {
		return "Not locked"
	}
	return fmt.Sprintf("App locked by %s, running %s. Acquired in %s",
		l.Owner,
		l.Reason,
		l.AcquireDate.Format(time.RFC3339),
	)
}

func (l *AppLock) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Locked      bool   `json:"Locked"`
		Reason      string `json:"Reason"`
		Owner       string `json:"Owner"`
		AcquireDate string `json:"AcquireDate"`
	}{
		Locked:      l.Locked,
		Reason:      l.Reason,
		Owner:       l.Owner,
		AcquireDate: l.AcquireDate.Format(time.RFC3339),
	})
}

func (l *AppLock) GetLocked() bool {
	return l.Locked
}

func (l *AppLock) GetReason() string {
	return l.Reason
}

func (l *AppLock) GetOwner() string {
	return l.Owner
}

func (l *AppLock) GetAcquireDate() time.Time {
	return l.AcquireDate
}

func (e ErrAppNotLocked) Error() string {
	return fmt.Sprintf("unable to lock app %q", e.App)
}
