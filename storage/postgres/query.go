// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"fmt"
	"strings"

	"github.com/tsuru/tsuru/types/query"
)

// jobColumns maps neutral field names to promoted SQL columns for the jobs table.
var jobColumns = map[string]string{
	"name":      "name",
	"teamowner": "team_owner",
	"owner":     "owner",
	"pool":      "pool",
	"tags":      "tags",
}

// translateJobQuery builds a SQL WHERE fragment (without the WHERE keyword) and
// the positional args. Returns ("", nil) for an empty query.
func translateJobQuery(q query.Query) (string, []any) {
	conds, args := buildConds(q, 0)
	if len(conds) == 0 {
		return "", nil
	}
	return strings.Join(conds, " AND "), args
}

func col(field string) string {
	if c, ok := jobColumns[field]; ok {
		return c
	}
	return fmt.Sprintf("doc->>'%s'", field)
}

func buildConds(q query.Query, start int) ([]string, []any) {
	var conds []string
	var args []any
	n := start
	next := func(v any) string { n++; args = append(args, v); return fmt.Sprintf("$%d", n) }

	for field, val := range q.Eq {
		conds = append(conds, fmt.Sprintf("%s = %s", col(field), next(val)))
	}
	for field, vals := range q.In {
		conds = append(conds, fmt.Sprintf("%s = ANY(%s)", col(field), next(vals)))
	}
	for field, pattern := range q.Regex {
		conds = append(conds, fmt.Sprintf("%s ~ %s", col(field), next(pattern)))
	}
	for field, vals := range q.All {
		// tags is a text[] column: contains-all via the @> operator.
		conds = append(conds, fmt.Sprintf("%s @> %s", col(field), next(vals)))
	}
	for _, sub := range q.Or {
		subConds, subArgs := buildConds(sub, n)
		if len(subConds) > 0 {
			conds = append(conds, "("+strings.Join(subConds, " OR ")+")")
			args = append(args, subArgs...)
			n += len(subArgs)
		}
	}
	return conds, args
}
