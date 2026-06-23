// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"strings"

	"github.com/tsuru/tsuru/types/query"
)

func processTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	processed := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if len(tag) > 0 && !seen[tag] {
			processed = append(processed, tag)
			seen[tag] = true
		}
	}
	return processed
}

// ToQuery converts the domain filter into a backend-neutral query.Query.
func (f *Filter) ToQuery() query.Query {
	q := query.Query{}
	if f == nil {
		return q
	}
	if f.Extra != nil {
		for field, values := range f.Extra {
			anyVals := make([]any, len(values))
			for i, v := range values {
				anyVals[i] = v
			}
			q.Or = append(q.Or, query.Query{In: map[string][]any{field: anyVals}})
		}
	}
	if f.Name != "" {
		q.Regex = map[string]string{"name": f.Name}
	}
	if f.TeamOwner != "" {
		q.Eq = ensureMap(q.Eq)
		q.Eq["teamowner"] = f.TeamOwner
	}
	if f.UserOwner != "" {
		q.Eq = ensureMap(q.Eq)
		q.Eq["owner"] = f.UserOwner
	}
	if f.Pool != "" {
		q.Eq = ensureMap(q.Eq)
		q.Eq["pool"] = f.Pool
	}
	if len(f.Pools) > 0 {
		anyVals := make([]any, len(f.Pools))
		for i, v := range f.Pools {
			anyVals[i] = v
		}
		q.In = map[string][]any{"pool": anyVals}
	}
	if tags := processTags(f.Tags); len(tags) > 0 {
		allVals := make([]any, len(tags))
		for i, v := range tags {
			allVals[i] = v
		}
		q.All = map[string][]any{"tags": allVals}
	}
	return q
}

func ensureMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
