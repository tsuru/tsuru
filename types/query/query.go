// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package query defines a backend-neutral query specification that storage
// adapters (mongodb, postgres) translate into their native query language.
package query

// Query is a conjunction (AND) of the populated clauses. Empty Query matches
// everything. Field names are the document field names as stored by the
// adapter.
type Query struct {
	// Eq matches field == value.
	Eq map[string]any
	// In matches field ∈ values.
	In map[string][]any
	// Regex matches field against an (unanchored) regular expression.
	Regex map[string]string
	// All matches array fields that contain every listed value.
	All map[string][]any
	// Or, when non-empty, is OR-combined with the rest of this Query.
	Or []Query
}

// IsEmpty reports whether the query has no clauses (matches everything).
func (q Query) IsEmpty() bool {
	return len(q.Eq) == 0 && len(q.In) == 0 && len(q.Regex) == 0 &&
		len(q.All) == 0 && len(q.Or) == 0
}
