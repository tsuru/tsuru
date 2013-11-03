// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package action

import (
	"errors"
	"github.com/globocom/tsuru/log"
	"sync"
)

// Result is the value returned by Forward. It is used in the call of the next
// action, and also when rolling back the actions.
type Result interface{}

// Forward is the function called by the pipeline executor in the forward
// phase.  It receives a FWContext instance, that contains the list of
// parameters given to the pipeline executor and the result of the previous
// action in the pipeline (which will be nil for the first action in the
// pipeline).
type Forward func(context FWContext) (Result, error)

// Backward is the function called by the pipeline executor when in the
// backward phase. It receives the context instance, that contains the list of
// parameters given to the pipeline executor and the result of the forward
// phase.
type Backward func(context BWContext)

// FWContext is the context used in calls to Forward functions (forward phase).
type FWContext struct {
	// Result of the previous action.
	Previous Result

	// List of parameters given to the executor.
	Params []interface{}
}

// BWContext is the context used in calls to Backward functions (backward
// phase).
type BWContext struct {
	// Result of the forward phase (for the current action).
	FWResult Result

	// List of parameters given to the executor.
	Params []interface{}
}

// Action defines actions that should be . It is composed of two functions:
// Forward and Backward.
//
// Each action should do only one thing, and do it well. All information that
// is needed to undo the action should be returned by the Forward function.
type Action struct {
	// Name is the action name. Used by the log.
	Name string

	// Function that will be invoked in the forward phase. This value
	// cannot be nil.
	Forward Forward

	// Function that will be invoked in the backward phase. For actions
	// that are not undoable, this attribute should be nil.
	Backward Backward

	// Minimum number of parameters that this action requires to run.
	MinParams int

	// Result of the action. Stored for use in the backward phase.
	result Result

	// mutex for the result
	rMutex sync.Mutex
}

// Pipeline is a list of actions. Each pipeline is atomic: either all actions
// are successfully executed, or none of them are. For that, it's fundamental
// that all actions are really small and atomic.
type Pipeline struct {
	actions []*Action
}

// NewPipeline creates a new pipeline instance with the given list of actions.
func NewPipeline(actions ...*Action) *Pipeline {
	return &Pipeline{actions: actions}
}

func (p *Pipeline) Result() Result {
	action := p.actions[len(p.actions)-1]
	action.rMutex.Lock()
	defer action.rMutex.Unlock()
	return action.result
}

// Execute executes the pipeline.
//
// The execution starts in the forward phase, calling the Forward function of
// all actions. If none of the Forward calls return error, the pipeline
// execution ends in the forward phase and is "committed".
//
// If any of the Forward call fail, the executor switches to the backward phase
// (roll back) and call the Backward function for each action completed. It
// does not call the Backward function of the action that has failed.
//
// After rolling back all completed actions, it returns the original error
// returned by the action that failed.
func (p *Pipeline) Execute(params ...interface{}) error {
	var (
		r   Result
		err error
	)
	if len(p.actions) == 0 {
		return errors.New("No actions to execute.")
	}
	fwCtx := FWContext{Params: params}
	for i, a := range p.actions {
		log.Debugf("[pipeline] running the Forward for the %s action", a.Name)
		if a.Forward == nil {
			err = errors.New("All actions must define the forward function.")
		} else if len(fwCtx.Params) < a.MinParams {
			err = errors.New("Not enough parameters to call Action.Forward.")
		} else {
			r, err = a.Forward(fwCtx)
			a.rMutex.Lock()
			a.result = r
			a.rMutex.Unlock()
			fwCtx.Previous = r
		}
		if err != nil {
			log.Debugf("[pipeline] error running the Forward for the %s action - %s", a.Name, err.Error())
			p.rollback(i-1, params)
			return err
		}
	}
	return nil
}

func (p *Pipeline) rollback(index int, params []interface{}) {
	bwCtx := BWContext{Params: params}
	for i := index; i >= 0; i-- {
		log.Debugf("[pipeline] running Backward for %s action", p.actions[i].Name)
		if p.actions[i].Backward != nil {
			bwCtx.FWResult = p.actions[i].result
			p.actions[i].Backward(bwCtx)
		}
	}
}
