// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"testing"
)

func TestFilterToQueryNil(t *testing.T) {
	var f *Filter
	if !f.ToQuery().IsEmpty() {
		t.Fatal("nil filter should produce empty query")
	}
}

func TestFilterToQueryFields(t *testing.T) {
	f := &Filter{Name: "web", TeamOwner: "t1", Pool: "p1", Tags: []string{"a", "a", "b"}}
	q := f.ToQuery()
	if q.Regex["name"] != "web" {
		t.Fatalf("name regex: got %q", q.Regex["name"])
	}
	if q.Eq["teamowner"] != "t1" {
		t.Fatalf("teamowner: got %v", q.Eq["teamowner"])
	}
	if q.Eq["pool"] != "p1" {
		t.Fatalf("pool: got %v", q.Eq["pool"])
	}
	if len(q.All["tags"]) != 2 {
		t.Fatalf("tags should be deduped to 2, got %v", q.All["tags"])
	}
}

func TestFilterToQueryPools(t *testing.T) {
	f := &Filter{Pools: []string{"p1", "p2"}}
	q := f.ToQuery()
	if len(q.In["pool"]) != 2 {
		t.Fatalf("pools In: got %v", q.In["pool"])
	}
}
