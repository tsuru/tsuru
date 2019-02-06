// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	tmpDir string
	env    *Environment
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	var err error
	s.tmpDir, err = ioutil.TempDir("", "tsuru-integration")
	c.Assert(err, check.IsNil)
	log.Printf("Using INTEGRATION HOME: %v", s.tmpDir)
	err = os.Setenv("HOME", s.tmpDir)
	c.Assert(err, check.IsNil)
	err = os.Setenv("TSURU_DISABLE_COLORS", "1")
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	err := os.RemoveAll(s.tmpDir)
	c.Assert(err, check.IsNil)
}

func retry(timeout time.Duration, fn func() bool) bool {
	timeoutTimer := time.After(timeout)
	for {
		if fn() {
			return true
		}
		select {
		case <-time.After(5 * time.Second):
		case <-timeoutTimer:
			return false
		}
	}
}

type resultTable struct {
	raw    string
	rows   [][]string
	header []string
}

func (r *resultTable) parse() {
	lines := strings.Split(r.raw, "\n")
	for _, line := range lines {
		if len(line) == 0 || line[0] != '|' {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if r.header == nil {
			r.header = append(r.header, parts...)
			continue
		}
		if len(parts) != len(r.header) {
			continue
		}
		if parts[0] == "" {
			for i := range r.rows[len(r.rows)-1] {
				if parts[i] == "" {
					continue
				}
				r.rows[len(r.rows)-1][i] += "\n" + parts[i]
			}
		} else {
			r.rows = append(r.rows, parts)
		}
	}
}

var dnsValidRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func slugifyName(name string) string {
	return strings.ToLower(dnsValidRegex.ReplaceAllString(name, "-"))
}
