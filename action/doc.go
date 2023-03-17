// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package action implements atomic pipeline execution of actions. A pipeline is a
set of actions that must be executed atomically: either all occur, or nothing
occur.

The pipeline executor has two possible phases: forward and backward. Whenever a
caller invoke the Execute method, the pipeline starts the forward phase. If any
action fail, then the executor enter in the backward phase and rolls back all
completed actions in that pipeline.

Each action contains two functions, matching the two executor phases: Forward
and Backward.

A pipeline is composed of a list of actions. For each action execution, the
executor will provide two possible contexts, based on the current phase of the
executor:

  - in forward phase, the Forward function will receive a FWContext, that
    contains a list of parameters passed to executor in the Execute() call and
    the the result of the previous action (which will be nil for the first action
    in the pipeline);

  - in backward phase, the Backward function will receive a BWContext, that
    also contains the list of parameters given to the executor, but instead of
    the previous result, it receives the result of the forward phase of the
    current action.

Besides the Context, the Backward function will also receive the result of the
Forward call.

For more details, check the documentation of the Execute method in the Pipeline
type.
*/
package action
