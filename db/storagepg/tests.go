// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagepg

import (
	"context"
	"errors"
	"testing"
)

// ClearAllTables truncates every adapter table. Tests only.
func ClearAllTables() error {
	if !testing.Testing() {
		return errors.New("ClearAllTables should only be used in tests")
	}
	p, err := Pool()
	if err != nil {
		return err
	}
	_, err = p.Exec(context.Background(), "TRUNCATE TABLE jobs")
	return err
}
