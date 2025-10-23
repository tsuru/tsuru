// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package action

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

type OnErrorFunc func(FWContext, error)

// FWContext is the context used in calls to Forward functions (forward phase).
type FWContext struct {
	Context context.Context
	// Result of the previous action.
	Previous Result

	// List of parameters given to the executor.
	Params []interface{}
}

// BWContext is the context used in calls to Backward functions (backward
// phase).
type BWContext struct {
	Context context.Context
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

	// Function that will be invoked after some failure occurured in the
	// Forward phase of this same action.
	OnError OnErrorFunc

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

var (
	ErrPipelineNoActions      = errors.New("No actions to execute.")
	ErrPipelineForwardMissing = errors.New("All actions must define the forward function.")
	ErrPipelineFewParameters  = errors.New("Not enough parameters to call Action.Forward.")
)

// NewPipeline creates a new pipeline instance with the given list of actions.
func NewPipeline(actions ...*Action) *Pipeline {
	// Actions are usually global functions, copying them
	// guarantees each copy has an isolated Result.
	newActions := make([]*Action, len(actions))
	for i, action := range actions {
		newAction := &Action{
			Name:      action.Name,
			Forward:   action.Forward,
			Backward:  action.Backward,
			MinParams: action.MinParams,
			OnError:   action.OnError,
		}
		newActions[i] = newAction
	}
	return &Pipeline{actions: newActions}
}

// Result returns the result of the last action.
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
// If any of the Forward calls fails, the executor switches to the backward phase
// (roll back) and call the Backward function for each action completed. It
// does not call the Backward function of the action that has failed.
//
// After rolling back all completed actions, it returns the original error
// returned by the action that failed.
func (p *Pipeline) Execute(ctx context.Context, params ...interface{}) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var r Result
	if len(p.actions) == 0 {
		return ErrPipelineNoActions
	}
	fwCtx := FWContext{Params: params}
	var i int
	var a *Action
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[pipeline] PANIC running the Forward for the %s action - %v", a.Name, r)
			debug.PrintStack()
			log.Errorf("[pipeline] PANIC STACK END")
			err = fmt.Errorf("panic running the Forward for the %s action: %v", a.Name, r)
			if a.OnError != nil {
				a.OnError(fwCtx, err)
			}
			p.rollback(ctx, i-1, params)
		}
	}()
	tracer := otel.Tracer("tsuru/action")
	for i, a = range p.actions {
		log.Debugf("[pipeline] running the Forward for the %s action", a.Name)
		actionCtx, span := tracer.Start(ctx, "Action forward "+a.Name)
		if a.Forward == nil {
			err = ErrPipelineForwardMissing
		} else if len(fwCtx.Params) < a.MinParams {
			err = ErrPipelineFewParameters
		} else {
			fwCtx.Context = actionCtx
			r, err = a.Forward(fwCtx)
			a.rMutex.Lock()
			a.result = r
			a.rMutex.Unlock()
			fwCtx.Previous = r
		}

		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
			span.End()

			log.Errorf("[pipeline] error running the Forward for the %s action - %s", a.Name, err)
			if a.OnError != nil {
				a.OnError(fwCtx, err)
			}
			p.rollback(ctx, i-1, params)
			return err
		}
		span.End()
	}
	return nil
}

func (p *Pipeline) rollback(ctx context.Context, index int, params []interface{}) {
	tracer := otel.Tracer("tsuru/action")
	bwCtx := BWContext{Params: params}
	for i := index; i >= 0; i-- {

		log.Debugf("[pipeline] running Backward for %s action", p.actions[i].Name)
		if p.actions[i].Backward != nil {
			actionCtx, span := tracer.Start(ctx, "Action backward "+p.actions[i].Name)
			bwCtx.Context = tsuruNet.WithoutCancel(actionCtx)

			bwCtx.FWResult = p.actions[i].result
			p.actions[i].Backward(bwCtx)

			span.End()
		}
	}
}
