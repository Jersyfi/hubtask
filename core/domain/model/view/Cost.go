// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import "strconv"

// The cost estimate (H-08, multi-tenancy.md §4): the rejection of obviously unaffordable
// queries, on top of the depth and node caps that were always the floor. Like them it is part
// of the grammar, not a configuration value - a query a client may send is a query every
// installation has to be able to plan.
//
// The model is deliberately crude. It does not predict a plan; it prices the shapes that are
// known to be expensive - text scans, list fans, negations the indexes cannot serve - and
// refuses the combinations no honest board query produces. An expensive-but-legal filter stays
// slow and finite under the statement timeout (ADR-0026); this refuses the ones that were never
// going to finish usefully at all.
const (
	// MaxFilterCost is the ceiling, in the units below. Fifty plain comparisons fit exactly -
	// the node cap and the cost cap agree about the largest boring query - while a filter of
	// text scans hits the wall at ten.
	MaxFilterCost = 50

	// costPlain is a comparison an index can serve: EQ, the ranges, IS_NULL.
	costPlain = 1
	// costList is a list operator: one comparison per value it fans out to, priced up front.
	costListPerValue = 1
	// costText is a text scan: CONTAINS and MATCHES walk what no btree serves.
	costText = 5
	// costStartsWith sits between: a prefix can use an index, but only sometimes.
	costStartsWith = 2
	// costNegation is what NOT multiplies its subtree by: an inverted predicate reads
	// everything the positive one would have skipped.
	costNegation = 2
)

// Cost prices one parsed tree. Priced after the grammar, so the tree is already bounded - the
// estimate is at most a few thousand and cannot overflow anything.
func Cost(node *Node) int {
	if node == nil {
		return 0
	}
	switch {
	case node.Op == OpNot:
		total := 0
		for i := range node.Nodes {
			total += Cost(&node.Nodes[i])
		}
		return total * costNegation
	case node.Op.Combines():
		total := 0
		for i := range node.Nodes {
			total += Cost(&node.Nodes[i])
		}
		return total
	case node.Op == OpContains || node.Op == OpMatches:
		return costText
	case node.Op == OpStartsWith:
		return costStartsWith
	case node.Op == OpIn || node.Op == OpNotIn ||
		node.Op == OpContainsAny || node.Op == OpContainsAll:
		if len(node.Values) == 0 {
			return costPlain
		}
		return costListPerValue * len(node.Values)
	default:
		return costPlain
	}
}

// checkCost refuses a tree past the ceiling, with the estimate and the ceiling in the answer so
// a client can see how far over it is.
func checkCost(node *Node, path string) error {
	cost := Cost(node)
	if cost <= MaxFilterCost {
		return nil
	}
	return fieldError(path, "query.filter_too_expensive", map[string]string{
		"cost":    strconv.Itoa(cost),
		"maximum": strconv.Itoa(MaxFilterCost),
	})
}
