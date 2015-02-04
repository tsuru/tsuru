// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmdtest

import "errors"

// FakeGuesser represents a fake implementation of the Guesser described in the
// cmd package.
type FakeGuesser struct {
	Name    string
	guesses []string
}

// HasGuess checks whether a guess call has been recorded with the given path.
func (f *FakeGuesser) HasGuess(path string) bool {
	for _, g := range f.guesses {
		if g == path {
			return true
		}
	}
	return false
}

func (f *FakeGuesser) GuessName(path string) (string, error) {
	f.guesses = append(f.guesses, path)
	return f.Name, nil
}

// FailingFakeGuesser is a guesser that fails in the GuessName call, with the
// given error message.
type FailingFakeGuesser struct {
	FakeGuesser
	ErrorMessage string
}

func (f *FailingFakeGuesser) GuessName(path string) (string, error) {
	f.FakeGuesser.GuessName(path)
	return "", errors.New(f.ErrorMessage)
}
