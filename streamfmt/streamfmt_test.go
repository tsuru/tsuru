// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamfmt

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSection(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Starting deploy", "---- Starting deploy ----"},
		{"", "----  ----"},
		{"Build", "---- Build ----"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, Section(tt.input))
		})
	}
}

func TestAction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Building image", " ---> Building image"},
		{"", " ---> "},
		{"Step 1", " ---> Step 1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, Action(tt.input))
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"deployment failed", "**** DEPLOYMENT FAILED ****"},
		{"Mixed Case Error", "**** MIXED CASE ERROR ****"},
		{"", "****  ****"},
		{"ALREADY UPPER", "**** ALREADY UPPER ****"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, Error(tt.input))
		})
	}
}

func TestSectionf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "with args",
			format:   "Deploying app %s to pool %s",
			args:     []interface{}{"myapp", "prod"},
			expected: "---- Deploying app myapp to pool prod ----",
		},
		{
			name:     "no args",
			format:   "Simple message",
			args:     nil,
			expected: "---- Simple message ----",
		},
		{
			name:     "with number",
			format:   "Version %d",
			args:     []interface{}{42},
			expected: "---- Version 42 ----",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, Sectionf(tt.format, tt.args...))
		})
	}
}

func TestActionf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "with args",
			format:   "Step %d of %d",
			args:     []interface{}{1, 5},
			expected: " ---> Step 1 of 5",
		},
		{
			name:     "no args",
			format:   "Simple action",
			args:     nil,
			expected: " ---> Simple action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, Actionf(tt.format, tt.args...))
		})
	}
}

func TestErrorf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "with args",
			format:   "failed to deploy %s: %s",
			args:     []interface{}{"myapp", "timeout"},
			expected: "**** FAILED TO DEPLOY MYAPP: TIMEOUT ****",
		},
		{
			name:     "no args",
			format:   "simple error",
			args:     nil,
			expected: "**** SIMPLE ERROR ****",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, Errorf(tt.format, tt.args...))
		})
	}
}

func TestFprintSectionf(t *testing.T) {
	var buf bytes.Buffer
	FprintSectionf(&buf, "Deploy %s", "myapp")
	require.Equal(t, "---- Deploy myapp ----", buf.String())
}

func TestFprintActionf(t *testing.T) {
	var buf bytes.Buffer
	FprintActionf(&buf, "Building %s", "image")
	require.Equal(t, " ---> Building image", buf.String())
}

func TestFprintErrorf(t *testing.T) {
	var buf bytes.Buffer
	FprintErrorf(&buf, "failed: %s", "error")
	require.Equal(t, "**** FAILED: ERROR ****", buf.String())
}

func TestFprintlnSectionf(t *testing.T) {
	var buf bytes.Buffer
	FprintlnSectionf(&buf, "Deploy %s", "myapp")
	require.Equal(t, "---- Deploy myapp ----\n", buf.String())
}

func TestFprintlnSectionfNilWriter(t *testing.T) {
	require.NotPanics(t, func() {
		FprintlnSectionf(nil, "Deploy %s", "myapp")
	})
}

func TestFprintlnActionf(t *testing.T) {
	var buf bytes.Buffer
	FprintlnActionf(&buf, "Building %s", "image")
	require.Equal(t, " ---> Building image\n", buf.String())
}

func TestFprintlnActionfNilWriter(t *testing.T) {
	require.NotPanics(t, func() {
		FprintlnActionf(nil, "Building %s", "image")
	})
}

func TestFprintlnErrorf(t *testing.T) {
	var buf bytes.Buffer
	FprintlnErrorf(&buf, "failed: %s", "error")
	require.Equal(t, "**** FAILED: ERROR ****\n", buf.String())
}

func TestFprintlnErrorfNilWriter(t *testing.T) {
	require.NotPanics(t, func() {
		FprintlnErrorf(nil, "failed: %s", "error")
	})
}

func TestConstants(t *testing.T) {
	require.Equal(t, "---- ", SectionPrefix)
	require.Equal(t, " ----", SectionSuffix)
	require.Equal(t, " ---> ", ActionPrefix)
	require.Equal(t, "**** ", ErrorPrefix)
	require.Equal(t, " ****", ErrorSuffix)
}
