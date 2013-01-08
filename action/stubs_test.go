// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package action

import "errors"

var helloAction = Action{
	Forward: func(ctx FWContext) (Result, error) {
		return "success", nil
	},
	Backward: func(ctx BWContext) {
	},
}

var errorAction = Action{
	Forward: func(ctx FWContext) (Result, error) {
		return nil, errors.New("Failed to execute.")
	},
	Backward: func(ctx BWContext) {},
}

var unrollbackableAction = Action{
	Forward: func(ctx FWContext) (Result, error) {
		return nil, nil
	},
	Backward: nil,
}
