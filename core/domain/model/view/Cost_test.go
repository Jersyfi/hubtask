// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// leafDocument builds one CONTAINS leaf - the priciest shape a filter has.
func containsLeaf() map[string]any {
	return map[string]any{"op": "CONTAINS", "field": "title", "value": "needle"}
}

// The boundary, documented and held: an affordable filter passes, an obviously unaffordable one
// is refused before it runs, with the estimate and the ceiling in the answer (H-08,
// multi-tenancy.md §4).
func TestTheCostCeilingRefusesTheUnaffordableAndOnlyIt(t *testing.T) {
	affordable := make([]any, 0, 10)
	for range 10 {
		affordable = append(affordable, containsLeaf())
	}
	if _, err := view.ParseFilter(map[string]any{"op": "AND", "nodes": affordable}, "/filter"); err != nil {
		t.Fatalf("ten text scans (cost 50) were refused at the ceiling of 50: %v", err)
	}

	unaffordable := append(affordable, containsLeaf())
	_, err := view.ParseFilter(map[string]any{"op": "AND", "nodes": unaffordable}, "/filter")
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "query.filter_too_expensive" {
		t.Fatalf("eleven text scans answered %v, want query.filter_too_expensive", err)
	}
	if domainErr.Params["cost"] != "55" || domainErr.Params["maximum"] != "50" {
		t.Errorf("params %v", domainErr.Params)
	}
}

// The weights, pinned: plain comparisons are cheap, text scans dear, a negation doubles its
// subtree, a list costs its fan-out. Fifty plain comparisons fit exactly - the node cap and the
// cost cap agree about the largest boring query.
func TestTheWeightsPriceTheKnownShapes(t *testing.T) {
	plain := &view.Node{Op: view.OpEq}
	if view.Cost(plain) != 1 {
		t.Errorf("a plain comparison costs %d", view.Cost(plain))
	}
	scan := &view.Node{Op: view.OpContains}
	if view.Cost(scan) != 5 {
		t.Errorf("a text scan costs %d", view.Cost(scan))
	}
	prefix := &view.Node{Op: view.OpStartsWith}
	if view.Cost(prefix) != 2 {
		t.Errorf("a prefix costs %d", view.Cost(prefix))
	}
	list := &view.Node{Op: view.OpIn, Values: make([]view.Value, 30)}
	if view.Cost(list) != 2 {
		t.Errorf("a thirty-value list costs %d", view.Cost(list))
	}
	// The grammar's own MaxValues stays affordable alone: a full board selection is legitimate.
	full := &view.Node{Op: view.OpIn, Values: make([]view.Value, 100)}
	if view.Cost(full) != 6 {
		t.Errorf("a full list costs %d", view.Cost(full))
	}
	negated := &view.Node{Op: view.OpNot, Nodes: []view.Node{{Op: view.OpContains}}}
	if view.Cost(negated) != 10 {
		t.Errorf("a negated scan costs %d", view.Cost(negated))
	}

	// The largest boring query: fifty plain comparisons, exactly at both caps.
	wide := &view.Node{Op: view.OpAnd, Nodes: make([]view.Node, 0, 49)}
	for range 49 {
		wide.Nodes = append(wide.Nodes, view.Node{Op: view.OpEq})
	}
	if view.Cost(wide) != 49 {
		t.Errorf("the wide plain query costs %d", view.Cost(wide))
	}
}

// A saved view is a filter too: an unaffordable one is refused at save, not discovered at run.
func TestAnUnaffordableFilterCannotBeSaved(t *testing.T) {
	nodes := make([]any, 0, 11)
	for range 11 {
		nodes = append(nodes, containsLeaf())
	}
	_, err := view.ParseFilter(map[string]any{"op": "OR", "nodes": nodes}, "/query/filter")
	if err == nil || !strings.Contains(err.Error(), "filter_too_expensive") {
		t.Errorf("answer %v", err)
	}
}
