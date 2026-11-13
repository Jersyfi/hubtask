// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// Asked is what a use case learnt of the asker for a key the statement cannot answer alone: whose
// time `time_logged` counts and which entries are `critical`. The permission questions behind it
// are the application layer's (rule 2); the statement only binds the answer.
type Asked struct {
	// Sight is whose time a time_logged leaf counts.
	Sight TimeSight
	// Critical are the entries on the critical path of the plan the query reaches.
	Critical []shared.ID
}

// Needs is what a tree asks of the use case before it can be compiled.
type Needs struct {
	Time     bool
	Critical bool
	// Groups are the group placeholders the tree names, each once.
	Groups []Placeholder
}

// Any reports whether the tree needs anything.
func (n Needs) Any() bool { return n.Time || n.Critical || len(n.Groups) > 0 }

// NeedsOf walks a tree, resolved or not, for what it asks.
func NeedsOf(node *Node) Needs {
	var needs Needs
	if node == nil {
		return needs
	}
	seen := map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		if !n.IsLeaf() {
			for _, child := range n.Nodes {
				walk(child)
			}
			return
		}
		switch n.Field.Name {
		case FieldTimeLogged:
			needs.Time = true
		case FieldCritical:
			needs.Critical = true
		}
		for _, value := range n.Values {
			kind := value.Placeholder.Kind
			if (kind == PlaceholderMyGroups || kind == PlaceholderGroup) && !seen[value.Placeholder.GroupKey()] {
				seen[value.Placeholder.GroupKey()] = true
				needs.Groups = append(needs.Groups, value.Placeholder)
			}
		}
	}
	walk(*node)
	return needs
}

// Answer fills every time_logged and critical leaf with what the use case learnt; a tree without
// either comes back unchanged. A critical leaf becomes a question of membership in the path:
// `critical = true` keeps the entries on it, `critical = false` (or NEQ true) the others.
func (n Node) Answer(asked Asked) Node {
	if !n.IsLeaf() {
		answered := Node{Op: n.Op, Nodes: make([]Node, 0, len(n.Nodes))}
		for _, child := range n.Nodes {
			answered.Nodes = append(answered.Nodes, child.Answer(asked))
		}
		return answered
	}
	switch n.Field.Name {
	case FieldTimeLogged, FieldCritical:
		answer := asked
		n.Asked = &answer
	}
	return n
}

// CriticalWanted reports whether a critical leaf keeps the entries on the path.
func (n Node) CriticalWanted() bool {
	on := len(n.Values) == 1 && n.Values[0].Bool
	if n.Op == OpNeq {
		return !on
	}
	return on
}
