// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// inherited is a QUERY widget's document with the dashboard's filter joined in (§6.5): the
// filter's top-level conditions on fields the widget released are left out, the rest is AND-ed
// with the widget's own filter or line. Both are read by the query's one parser, so the result
// is a tree the use case checks again and binds as parameters (rule 9) - no text is spliced.
// Without a filter the document goes on as it was stored.
func inherited(in usecase.Input, d domain.Dashboard, w domain.Widget) (usecase.Input, error) {
	if d.Filter == "" {
		return in, nil
	}
	q, err := view.ParseQuery(d.Filter, "/filter")
	if err != nil {
		return nil, err
	}
	shared, err := q.EntryFilter("/filter")
	if err != nil {
		return nil, err
	}
	shared = released(shared, w.Released)
	if shared == nil {
		return in, nil
	}
	out := usecase.Input{}
	for k, v := range in {
		out[k] = v
	}
	own := out["filter"]
	if line, ok := out["q"].(string); ok {
		delete(out, "q")
		parsed, err := view.ParseQuery(line, "/q")
		if err != nil {
			return nil, err
		}
		if own, err = parsed.EntryFilter("/q"); err != nil {
			return nil, err
		}
		if len(parsed.Sort) > 0 {
			out["sort"] = parsed.Sort
		}
		if parsed.IncludeArchived {
			out["include_archived"] = true
		}
		if parsed.IncludeTrashed {
			out["include_trashed"] = true
		}
	}
	if own == nil {
		out["filter"] = shared
		return out, nil
	}
	out["filter"] = map[string]any{"op": "AND", "nodes": []any{shared, own}}
	return out, nil
}

// released drops the top-level conditions on the given fields; a group is kept whole, because
// what it ORs together is one condition.
func released(node any, fields []string) any {
	if len(fields) == 0 {
		return node
	}
	gone := map[string]bool{}
	for _, f := range fields {
		gone[f] = true
	}
	doc, ok := node.(map[string]any)
	if !ok {
		return node
	}
	if field, leaf := doc["field"].(string); leaf {
		if gone[field] {
			return nil
		}
		return node
	}
	if doc["op"] != "AND" {
		return node
	}
	nodes, _ := doc["nodes"].([]any)
	kept := make([]any, 0, len(nodes))
	for _, n := range nodes {
		if child, ok := n.(map[string]any); ok {
			if field, leaf := child["field"].(string); leaf && gone[field] {
				continue
			}
		}
		kept = append(kept, n)
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	}
	return map[string]any{"op": "AND", "nodes": kept}
}
