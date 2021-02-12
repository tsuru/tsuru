// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package errors provides facilities with error handling.
package errors

import (
	"fmt"
	"strings"
)

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

func (e *HTTP) StatusCode() int {
	return e.Code
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

func (m *MultiError) Append(me *MultiError) {
	m.errors = append(m.errors, me.errors...)
}

func (m *MultiError) Len() int {
	return len(m.errors)
}

func (m *MultiError) ToError() error {
	if m.Len() == 0 {
		return nil
	}
	if m.Len() == 1 {
		return m.errors[0]
	}
	return m
}

func (m *MultiError) Error() string {
	if m.Len() == 0 {
		return "multi error created but no errors added"
	}
	if m.Len() == 1 {
		return fmt.Sprintf("%+v", m.errors[0])
	}
	msg := fmt.Sprintf("multiple errors reported (%d):\n", len(m.errors))
	for i, err := range m.errors {
		msg += fmt.Sprintf("error #%d: %+v\n", i, err)
	}
	return msg
}

func (m *MultiError) Format(s fmt.State, verb rune) {
	if m.Len() == 0 {
		return
	}
	fmtString := "%"
	if s.Flag('+') {
		fmtString += "+"
	}
	if s.Flag('#') {
		fmtString += "#"
	}
	fmtString += string(verb)
	if m.Len() == 1 {
		fmt.Fprintf(s, fmtString, m.errors[0])
		return
	}
	errors := make([]string, len(m.errors))
	for i, err := range m.errors {
		errors[i] = fmt.Sprintf("error %d: "+fmtString, i, err)
	}
	fmt.Fprintf(s, "multiple errors reported (%d): %s", m.Len(), strings.Join(errors, " - "))
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
