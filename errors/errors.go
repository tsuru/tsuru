// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package errors provides facilities with error handling.
package errors

import "fmt"

// HTTP represents an HTTP error. It implements the error interface.
//
// Each HTTP error has a Code and a message explaining what went wrong.
type HTTP struct {
	// Status code.
	Code int

	// Message explaining what went wrong.
	Message string
}

func (e *HTTP) Error() string {
	return e.Message
}

// ValidationError is an error implementation used whenever a validation
// failure occurs.
type ValidationError struct {
	Message string
}

func (err *ValidationError) Error() string {
	return err.Message
}

type ConflictError ValidationError

func (err *ConflictError) Error() string {
	return err.Message
}

type NotAuthorizedError ValidationError

func (err *NotAuthorizedError) Error() string {
	return err.Message
}

type MultiError struct {
	errors []error
}

func NewMultiError(errs ...error) *MultiError {
	return &MultiError{errors: errs}
}

func (m *MultiError) Add(err error) {
	m.errors = append(m.errors, err)
}

func (m *MultiError) Len() int {
	return len(m.errors)
}

func (m *MultiError) Error() string {
	if len(m.errors) == 0 {
		return "multi error created but no errors added"
	}
	if len(m.errors) == 1 {
		return fmt.Sprintf("%+v", m.errors[0])
	}
	msg := fmt.Sprintf("multiple errors reported (%d):\n", len(m.errors))
	for i, err := range m.errors {
		msg += fmt.Sprintf("error #%d: %+v\n", i, err)
	}
	return msg
}

type CompositeError struct {
	Base    error
	Message string
}

func (err *CompositeError) Error() string {
	if err.Base == nil {
		return err.Message
	}
	return fmt.Sprintf("%s Caused by: %s", err.Message, err.Base.Error())
}
