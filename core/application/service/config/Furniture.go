// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"maps"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A workspace's and a hub's furniture as sections of their documents (config-document.md §1):
// the saved views shared where they live - a queue is one - and the automation rules there - a
// macro is one -, each by its stable key (migration 0286). A view's anchor, a `collection_id` it
// filters on and a rule step's container are named by the hub's or collection's handle, and in a
// hub's own document the hub itself by `@hub`, so a hub's document applies to a second hub
// unchanged. An identifier no handle is known for - a place the reader does not reach - stays as
// it is, and a handle no place holds is refused by the use case, never guessed.
const (
	sectionViews = "views"
	sectionRules = "rules"
	// placeHub is the hub whose own document this is.
	placeHub = "@hub"
	// placeLiteral marks a place named inside a rule condition's expression, where nothing but
	// the mark tells a place from text: `collection.id == 'place:network'`.
	placeLiteral = "place:"
	// chained is the identifier an operation takes from the one before it in the same apply - a
	// rule switched on right after it was written.
	chained = "00000000-0000-0000-0000-000000000000"
	// rulePages bounds the rule listing an export pages through: a workspace with more rules than
	// that is refused its document rather than handed one that leaves rules out, which an apply
	// would read as removals.
	rulePages    = 50
	rulePageSize = 200
)

// places maps the workspace's hubs and collections to their handles and back, as the reader
// reaches them; hub is the hub whose own document is read, empty for the workspace's.
type places struct {
	key map[string]string
	id  map[string]string
	hub string
	// children and order are the hub's own collections, for its document's collection blocks.
	children map[string]bool
	order    []string
}

// out names a place by its key: `@hub` for the document's own hub, a handle, or the identifier
// where none is known.
func (p places) out(v any) any {
	s, isText := v.(string)
	if !isText {
		return v
	}
	if p.hub != "" && s == p.hub {
		return placeHub
	}
	if key, known := p.key[s]; known {
		return key
	}
	return v
}

// in is the identifier a key names, or the value as it came.
func (p places) in(v any) any {
	s, isText := v.(string)
	if !isText {
		return v
	}
	if s == placeHub && p.hub != "" {
		return p.hub
	}
	if id, known := p.id[s]; known {
		return id
	}
	return v
}

// furnitureSections are a document's views and rules: the workspace's own, or a hub's.
func furnitureSections(hub shared.ID) []section {
	scopeType, scopeID := "TENANT", ""
	viewsInput := usecase.Input{"workspace": true}
	if !hub.IsZero() {
		scopeType, scopeID = "HUB", hub.String()
		viewsInput = usecase.Input{"container_id": scopeID}
	}
	ruleAttributes := []string{"name", "enabled", "run_as", "trigger", "conditions", "actions", "throttle", "on_error"}
	return []section{
		{
			name: sectionViews, list: "ListSavedViews", listInput: viewsInput, rows: "data", key: "key",
			rowsOf: func(answer map[string]any) []any {
				rows, _ := answer["data"].([]any)
				var out []any
				for _, raw := range rows {
					row, _ := raw.(map[string]any)
					if text(row["sharing"]) == "SCOPE" && text(row["scope_type"]) == scopeType && text(row["scope_id"]) == scopeID {
						out = append(out, row)
					}
				}
				return out
			},
			attributes: viewAttributes, changeable: viewAttributes,
			create: "CreateSavedView", change: "UpdateSavedView", remove: "DeleteSavedView",
			locate: "view_id", locateFrom: "id",
			present: func(p places, block domain.Block) domain.Block {
				block["query"] = mapQuery(block["query"], p.out)
				return block
			},
			opsAt: func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
				return viewOps(now, change, row, change.Key, scopeType, scopeID)
			},
			hides: removing,
		},
		{
			name: sectionRules, list: "ListRules", listInput: usecase.Input{"size": rulePageSize}, rows: "data", key: "key",
			paged: true,
			rowsOf: func(answer map[string]any) []any {
				rows, _ := answer["data"].([]any)
				var out []any
				for _, raw := range rows {
					row, _ := raw.(map[string]any)
					scope, _ := row["scope"].(map[string]any)
					if text(scope["type"]) == scopeType && text(scope["id"]) == scopeID {
						out = append(out, row)
					}
				}
				return out
			},
			attributes: ruleAttributes, changeable: ruleAttributes,
			create: "CreateRule", change: "UpdateRule", remove: "DeleteRule",
			locate: "rule_id", locateFrom: "id",
			present: func(p places, block domain.Block) domain.Block {
				block["actions"] = mapActions(block["actions"], p.out)
				block["conditions"] = mapConditions(block["conditions"], p.literalOut)
				return block
			},
			opsAt: func(now current, change domain.BlockChange, row map[string]any) ([]operation, error) {
				return ruleOps(now, change, row, change.Key, scopeType, scopeID)
			},
			// A rule acts with somebody's rights, and its removal stops what people rely on.
			hides: removing, permissions: true,
		},
	}
}

// viewAttributes are what a shared view's block holds.
var viewAttributes = []string{"name", "layout", "query", "grouping", "visible_fields"}

// viewOps writes a shared view under key where scopeType and scopeID say, the places its query
// names back to identifiers.
func viewOps(now current, change domain.BlockChange, row map[string]any, key, scopeType, scopeID string) ([]operation, error) {
	path := change.Path()
	after := domain.Block{}
	for name, value := range change.After {
		after[name] = value
	}
	if query, held := after["query"]; held {
		after["query"] = mapQuery(query, now.Places.in)
	}
	switch change.Op {
	case domain.OpAdd:
		in := usecase.Input{"key": key, "scope_type": scopeType, "sharing": "SCOPE"}
		if scopeID != "" {
			in["scope_id"] = scopeID
		}
		for _, name := range viewAttributes {
			if value, held := after[name]; held {
				in[name] = value
			}
		}
		return []operation{{UseCase: "CreateSavedView", Path: path, Input: in}}, nil
	case domain.OpChange:
		in := usecase.Input{"view_id": row["id"]}
		for _, name := range change.Fields {
			// An attribute the document leaves out is left as it is.
			if value, held := after[name]; held {
				in[name] = value
			}
		}
		if len(in) == 1 {
			return nil, nil
		}
		return []operation{{UseCase: "UpdateSavedView", Path: path, Input: in}}, nil
	default:
		return []operation{{UseCase: "DeleteSavedView", Path: path, Input: usecase.Input{"view_id": row["id"]}}}, nil
	}
}

// ruleOps writes a rule under key through its use cases: written switched off, then switched on
// where the block says so, so the trail says both; a change of `enabled` is its own use case.
func ruleOps(now current, change domain.BlockChange, row map[string]any, key, scopeType, scopeID string) ([]operation, error) {
	path := change.Path()
	if change.Op == domain.OpRemove {
		return []operation{{UseCase: "DeleteRule", Path: path, Input: usecase.Input{"rule_id": row["id"]}}}, nil
	}
	after := domain.Block{}
	for name, value := range change.After {
		after[name] = value
	}
	if actions, held := after["actions"]; held {
		after["actions"] = mapActions(actions, now.Places.in)
	}
	if conditions, held := after["conditions"]; held {
		after["conditions"] = mapConditions(conditions, now.Places.literalIn)
	}
	enabled, _ := after["enabled"].(bool)
	if change.Op == domain.OpAdd {
		scope := map[string]any{"type": scopeType}
		if scopeID != "" {
			scope["id"] = scopeID
		}
		in := usecase.Input{"key": key, "scope": scope}
		for _, name := range []string{"name", "run_as", "trigger", "conditions", "actions", "throttle", "on_error"} {
			if value, held := after[name]; held {
				in[name] = value
			}
		}
		if text(in["run_as"]) == "" {
			// A rule a document brings acts as whoever applies it, unless it names somebody.
			in["run_as"] = now.Account.String()
		}
		ops := []operation{{UseCase: "CreateRule", Path: path, Input: in}}
		if enabled {
			ops = append(ops, operation{UseCase: "EnableRule", Path: path, Input: usecase.Input{"rule_id": chained}, Chain: "id"})
		}
		return ops, nil
	}
	var ops []operation
	in := usecase.Input{"rule_id": row["id"]}
	for _, name := range change.Fields {
		// An attribute the document leaves out is left as it is: a template's rule names no
		// `run_as`, the rule in force does.
		if value, held := after[name]; held && name != "enabled" {
			in[name] = value
		}
	}
	if len(in) > 1 {
		ops = append(ops, operation{UseCase: "UpdateRule", Path: path, Input: in})
	}
	if _, held := after["enabled"]; held && contains(change.Fields, "enabled") {
		useCase := "DisableRule"
		if enabled {
			useCase = "EnableRule"
		}
		ops = append(ops, operation{UseCase: useCase, Path: path, Input: usecase.Input{"rule_id": row["id"]}})
	}
	return ops, nil
}

// mapQuery names the places a view's query names - its anchor and a `collection_id` it filters
// on - through name, leaving everything else as it is.
func mapQuery(raw any, name func(any) any) any {
	query, isObject := raw.(map[string]any)
	if !isObject {
		return raw
	}
	out := map[string]any{}
	for k, v := range query {
		out[k] = v
	}
	if anchor, held := out["scope_container_id"]; held {
		out["scope_container_id"] = name(anchor)
	}
	if filter, held := out["filter"]; held {
		out["filter"] = mapFilter(filter, name)
	}
	return out
}

func mapFilter(raw any, name func(any) any) any {
	node, isObject := raw.(map[string]any)
	if !isObject {
		return raw
	}
	out := map[string]any{}
	for k, v := range node {
		out[k] = v
	}
	if nodes, held := out["nodes"].([]any); held {
		mapped := make([]any, len(nodes))
		for i, child := range nodes {
			mapped[i] = mapFilter(child, name)
		}
		out["nodes"] = mapped
	}
	if text(out["field"]) == "collection_id" {
		switch value := out["value"].(type) {
		case []any:
			mapped := make([]any, len(value))
			for i, v := range value {
				mapped[i] = name(v)
			}
			out["value"] = mapped
		default:
			out["value"] = name(value)
		}
	}
	return out
}

// literalOut is a condition's string literal as the document holds it: a place's identifier as
// its marked handle, anything else as it is.
func (p places) literalOut(literal string) string {
	if named, _ := p.out(literal).(string); named != literal {
		return placeLiteral + named
	}
	return literal
}

// literalIn is a condition's string literal as the rule holds it: a marked handle as the place's
// identifier. A handle no place holds is left marked; the plan refuses it (placeRefusals).
func (p places) literalIn(literal string) string {
	if handle, marked := strings.CutPrefix(literal, placeLiteral); marked {
		if id, _ := p.in(handle).(string); id != handle {
			return id
		}
	}
	return literal
}

// placeRefusals are the marked handles a rule block's conditions name that no place holds.
func (p places) placeRefusals(path string, block map[string]any) []shared.FieldError {
	var refusals []shared.FieldError
	mapConditions(block["conditions"], func(literal string) string {
		if handle, marked := strings.CutPrefix(literal, placeLiteral); marked && p.literalIn(literal) == literal {
			refusals = append(refusals, domain.Refusal("config.place_unknown", path, map[string]string{"handle": handle}))
		}
		return literal
	})
	return refusals
}

// mapConditions renames the string literals of a rule's condition expressions through name.
func mapConditions(raw any, name func(string) string) any {
	conditions, isList := raw.([]any)
	if !isList {
		return raw
	}
	out := make([]any, len(conditions))
	for i, item := range conditions {
		condition, isObject := item.(map[string]any)
		expr, isText := condition["expr"].(string)
		if !isObject || !isText {
			out[i] = item
			continue
		}
		copied := maps.Clone(condition)
		copied["expr"] = mapLiterals(expr, name)
		out[i] = copied
	}
	return out
}

// mapLiterals hands each quoted string literal of a CEL expression to name, the quotes and
// everything outside them kept. A literal holding an escape is passed as written, so it never
// equals an identifier or a marked handle; an unclosed quote ends the scan unchanged.
func mapLiterals(expr string, name func(string) string) string {
	var b strings.Builder
	for i := 0; i < len(expr); {
		quote := expr[i]
		if quote != '\'' && quote != '"' {
			b.WriteByte(quote)
			i++
			continue
		}
		end := i + 1
		for end < len(expr) && expr[end] != quote {
			if expr[end] == '\\' {
				end++
			}
			end++
		}
		if end >= len(expr) {
			b.WriteString(expr[i:])
			break
		}
		b.WriteByte(quote)
		b.WriteString(name(expr[i+1 : end]))
		b.WriteByte(quote)
		i = end + 1
	}
	return b.String()
}

// mapActions names the places a rule's steps name: a parameter for a collection, a container or
// a hub.
func mapActions(raw any, name func(any) any) any {
	actions, isList := raw.([]any)
	if !isList {
		return raw
	}
	out := make([]any, len(actions))
	for i, item := range actions {
		action, isObject := item.(map[string]any)
		if !isObject {
			out[i] = item
			continue
		}
		copied := map[string]any{}
		for k, v := range action {
			copied[k] = v
		}
		if params, held := action["params"].(map[string]any); held {
			mapped := map[string]any{}
			for k, v := range params {
				if strings.HasSuffix(k, "collection_id") || strings.HasSuffix(k, "container_id") || strings.HasSuffix(k, "hub_id") {
					v = name(v)
				}
				mapped[k] = v
			}
			copied["params"] = mapped
		}
		out[i] = copied
	}
	return out
}

// readPlaces lists the hubs and collections the reader reaches with their handles: the hubs, then
// each hub's collections, archived ones included.
func (d Desk) readPlaces(ctx context.Context, actor appshared.ActorContext, hub shared.ID) (places, error) {
	out := places{key: map[string]string{}, id: map[string]string{}, children: map[string]bool{}}
	if !hub.IsZero() {
		out.hub = hub.String()
	}
	var hubs []string
	err := d.eachPage(ctx, actor, "ListContainers", usecase.Input{"type": "HUB", "include_archived": true}, func(row map[string]any) {
		hubs = append(hubs, text(row["id"]))
		out.add(row)
	})
	if err != nil {
		return places{}, err
	}
	for _, id := range hubs {
		own := id == out.hub
		if err := d.eachPage(ctx, actor, "ListContainers", usecase.Input{"parent_id": id, "include_archived": true}, func(row map[string]any) {
			out.add(row)
			if child := text(row["id"]); own && child != "" && text(row["handle"]) != "" {
				out.children[child] = true
				out.order = append(out.order, child)
			}
		}); err != nil {
			return places{}, err
		}
	}
	return out, nil
}

func (p places) add(row map[string]any) {
	id, handle := text(row["id"]), text(row["handle"])
	if id == "" || handle == "" {
		return
	}
	p.key[id] = handle
	p.id[handle] = id
}

// eachPage walks a paged list use case to its end, bounded.
func (d Desk) eachPage(ctx context.Context, actor appshared.ActorContext, name string, in usecase.Input, each func(map[string]any)) error {
	cursor := ""
	for range rulePages {
		page := usecase.Input{"size": rulePageSize}
		for k, v := range in {
			page[k] = v
		}
		if cursor != "" {
			page["cursor"] = cursor
		}
		answer, err := d.Catalogue.Invoke(ctx, name, actor, page)
		if err != nil {
			return err
		}
		normal, err := normalise(map[string]any(answer))
		if err != nil {
			return err
		}
		rows, _ := normal.(map[string]any)["data"].([]any)
		for _, raw := range rows {
			if row, isObject := raw.(map[string]any); isObject {
				each(row)
			}
		}
		state, _ := normal.(map[string]any)["page"].(map[string]any)
		if more, _ := state["has_more"].(bool); !more {
			return nil
		}
		if cursor = text(state["next_cursor"]); cursor == "" {
			return nil
		}
	}
	return refused("config.section_too_large", name, nil)
}
