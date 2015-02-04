// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package validation provide utilities functions for data validation.
package validation

import "regexp"

func ValidateEmail(email string) bool {
	re := regexp.MustCompile(`^([^@\s]+)@((?:[-a-z0-9]+\.)+[a-z]{2,})$`)
	return re.MatchString(email)
}

// ValidateLength checks whether the given data match the given rules.
//
// It checks if the value has more or equal `min` chars and less or equal `max`
// chars. If you don't want to check both, just pass a zero value:
//
//     ValidateLength(value, 0, 100) // Checks if value has at most 100 characters
//     ValidateLength(value, 100, 0) // Checks if value has at least 100 characters
//     ValidateLength(value, 20, 100) // Checks if value has at least 20 characters and at most 100 characters
func ValidateLength(value string, min, max int) bool {
	l := len(value)
	if min > 0 && l < min {
		return false
	}
	if max > 0 && l > max {
		return false
	}
	return true
}
