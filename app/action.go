// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package app

// actions represents an action, with the given methods
// to forward and backward for the action.
type action interface {
	forward(app App) error
	backward(app App)
}

// Execute runs an action list. If an errors ocourrs
// Execute stops the execution for the actions and call
// the rollback for previous actions.
func Execute(a App, actions []action) {
	for index, action := range actions {
		err := action.forward(a)
		if err != nil {
			RollBack(a, actions, index)
		}
	}
}

// RollBack runs the rollback for the given actions.
func RollBack(a App, actions []action, index int) {
	for i := index; i >= 0; i-- {
		action := actions[i]
		action.backward(a)
	}
}
