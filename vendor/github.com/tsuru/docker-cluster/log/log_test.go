// Copyright 2014 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDefaultsToStdErr(t *testing.T) {
	defer func() {
		if val := recover(); val != nil {
			t.Fatalf("Expected not to panic, got: %#v", val)
		}
	}()
	Errorf("%s - %s - %d", "foo", "bar", 1)
}

func TestSetOutputNil(t *testing.T) {
	defer func() {
		if val := recover(); val != nil {
			t.Fatalf("Expected not to panic, got: %#v", val)
		}
	}()
	SetLogger(nil)
	Errorf("%s - %s - %d", "foo", "bar", 1)
}

func TestDebugf(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(log.New(&buf, "", 0))
	SetDebug(true)
	Debugf("%s - %s - %d", "foo", "bar", 1)
	expected := "[docker-cluster][debug] foo - bar - 1\n"
	if !strings.Contains(buf.String(), expected) {
		t.Fatalf("Expected log to be %q, got: %q", expected, buf.String())
	}
}

func TestDebugfWithoutDebug(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(log.New(&buf, "", 0))
	SetDebug(false)
	Debugf("%s - %s - %d", "foo", "bar", 1)
	expected := ""
	if !strings.Contains(buf.String(), expected) {
		t.Fatalf("Expected log to be %q, got: %q", expected, buf.String())
	}
}

func TestErrorf(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(log.New(&buf, "", 0))
	SetDebug(false)
	Errorf("%s - %s - %d", "foo", "bar", 1)
	expected := "[docker-cluster][error] foo - bar - 1\n"
	if !strings.Contains(buf.String(), expected) {
		t.Fatalf("Expected log to be %q, got: %q", expected, buf.String())
	}
}
