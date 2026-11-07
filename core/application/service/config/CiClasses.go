// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
)

// sectionCiClasses is the workspace's own configuration item classes (configuration.md §6). A
// shipped class is never changed, so it is no block; an archived one is none either, and removing
// a block archives its class: a class is never deleted while CIs may hold it.
const sectionCiClasses = "ci_classes"

// ciClassAttributes is what a block keeps of a class: its own part, never what it inherits.
var ciClassAttributes = []string{"parent", "name", "icon", "abstract", "attributes", "identifier_rules", "required_relations", "staleness_days"}

func ciClassesSection() section {
	return section{
		name: sectionCiClasses, module: module.Assets, list: "ListCiClasses", rows: "data", key: "key",
		attributes: ciClassAttributes,
		// DefineCiClass both makes and changes a class; ops writes it whole.
		create: "DefineCiClass", change: "DefineCiClass",
		block: ciClassBlock, ops: ciClassOps,
	}
}

// ciClassBlock reads a class's own part in the document's shape; a value left at its default is
// left out, so that a template's block reads back as it was written.
func ciClassBlock(row map[string]any) (string, domain.Block) {
	key := text(row["key"])
	if system, _ := row["system"].(bool); system {
		return "", nil
	}
	if archived, _ := row["archived"].(bool); archived {
		return "", nil
	}
	block := domain.Block{"parent": row["parent"], "name": row["name"]}
	if icon := text(row["icon"]); icon != "" {
		block["icon"] = icon
	}
	if abstract, _ := row["abstract"].(bool); abstract {
		block["abstract"] = true
	}
	if attributes := ciClassAttributesOf(row["attributes"]); len(attributes) > 0 {
		block["attributes"] = attributes
	}
	var rules []any
	for _, r := range rowsOfAny(row["identifier_rules"]) {
		if text(r["class"]) == key {
			rules = append(rules, map[string]any{"attributes": r["attributes"]})
		}
	}
	if len(rules) > 0 {
		block["identifier_rules"] = rules
	}
	var required []any
	for _, r := range rowsOfAny(row["required_relations"]) {
		if text(r["class"]) == key {
			required = append(required, map[string]any{"types": r["types"], "outgoing": r["outgoing"]})
		}
	}
	if len(required) > 0 {
		block["required_relations"] = required
	}
	if days := number(row["own_staleness_days"]); days > 0 {
		block["staleness_days"] = days
	}
	return key, block
}

func ciClassAttributesOf(v any) []any {
	var out []any
	for _, a := range rowsOfAny(v) {
		attribute := map[string]any{"key": a["key"], "kind": a["kind"], "need": a["need"]}
		if identifier, _ := a["identifier"].(bool); identifier {
			attribute["identifier"] = true
		}
		if options, _ := a["options"].([]any); len(options) > 0 {
			attribute["options"] = options
		}
		out = append(out, attribute)
	}
	return out
}

// ciClassOps defines the class whole from its block; a removal archives it as it stood.
func ciClassOps(change domain.BlockChange, _ map[string]any) ([]operation, error) {
	block, archived := change.After, false
	if change.Op == domain.OpRemove {
		block, archived = change.Before, true
	}
	in := usecase.Input{"key": change.Key, "archived": archived}
	for _, attribute := range ciClassAttributes {
		if value, held := block[attribute]; held {
			in[attribute] = value
		}
	}
	return []operation{{UseCase: "DefineCiClass", Path: change.Path(), Input: in}}, nil
}

// rowsOfAny reads a list of objects in the document's shapes or a use case's answer.
func rowsOfAny(v any) []map[string]any {
	var out []map[string]any
	switch list := v.(type) {
	case []any:
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
	case []map[string]any:
		out = list
	case []usecase.Output:
		for _, item := range list {
			out = append(out, map[string]any(item))
		}
	}
	return out
}

// number reads a whole number from a use case's answer or a decoded document.
func number(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
