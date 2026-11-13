// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"slices"
	"strconv"
	"strings"
)

// The objects a key asks about, as QueryKey.Objects and KindFilter.Kind name them.
const (
	ObjectEntry = "entry"
	ObjectAsset = "asset"
	ObjectPage  = "page"
	ObjectGoal  = "goal"
)

// The keys of one kind besides entries. The key is the field's name: no entry column stands
// behind them, so they are not in the entry catalogue, and a query of entries refuses them.
const (
	FieldAssetClass    = "asset.class"
	FieldAssetStage    = "asset.stage"
	FieldAssetOwner    = "asset.owner"
	FieldAssetLocation = "asset.location"
	FieldAssetTag      = "asset.tag"
	FieldPageVerified  = "page.verified"
	FieldPageOwner     = "page.owner"
	FieldGoalState     = "goal.state"
)

// The stages of an inventory item and the states of a goal, spelled here because the view
// package imports no product's model; the application's test holds them to the products'.
var (
	AssetStages = []string{"PLANNED", "ORDERED", "IN_STOCK", "IN_USE", "MAINTENANCE", "RETIRED", "DISPOSED"}
	GoalStates  = []string{"NONE", "ON_TRACK", "AT_RISK", "OFF_TRACK", "DONE", "DROPPED"}
)

var kindOperators = []Operator{OpEq, OpNeq, OpIn, OpNotIn}

var kindCatalogue = []Field{
	{Name: FieldAssetClass, Kind: KindString, Operators: kindOperators},
	{Name: FieldAssetStage, Kind: KindEnum, Operators: kindOperators, Values: AssetStages},
	{Name: FieldAssetOwner, Kind: KindID, Operators: append(slices.Clone(kindOperators), OpIsNull), Nullable: true},
	{Name: FieldAssetLocation, Kind: KindID, Operators: append(slices.Clone(kindOperators), OpIsNull), Nullable: true},
	{Name: FieldAssetTag, Kind: KindString, Operators: append(slices.Clone(kindOperators), OpIsNull), Nullable: true},
	{Name: FieldPageVerified, Kind: KindBool, Operators: []Operator{OpEq, OpNeq}},
	{Name: FieldPageOwner, Kind: KindID, Operators: append(slices.Clone(kindOperators), OpIsNull), Nullable: true},
	{Name: FieldGoalState, Kind: KindEnum, Operators: kindOperators, Values: GoalStates},
}

// KindFields are the keys of one kind, in the catalogue's order.
func KindFields() []Field { return slices.Clone(kindCatalogue) }

func kindField(name string) (Field, bool) {
	for _, f := range kindCatalogue {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// objectOf is the kind a field asks about.
func objectOf(name string) string {
	if _, ok := kindField(name); ok {
		kind, _, _ := strings.Cut(name, ".")
		return kind
	}
	return ObjectEntry
}

// queryField resolves a field the query text may name: an entry's, or a key of one kind.
func queryField(name, path string) (Field, error) {
	if f, ok := kindField(name); ok {
		return f, nil
	}
	return field(name, path)
}

// KindCondition is one key of a kind as its search binds it: the field is one of Values, or none
// where None; Negated turns the whole condition round. Values are as typed, an identifier, `@me`,
// an enum value, or `true`/`false`.
type KindCondition struct {
	Field   string
	Values  []string
	None    bool
	Negated bool
}

// KindFilter is the filter of a query read for the one kind it asks about: Kind is empty where the
// query holds no field, ObjectEntry where its fields are entries' (Query.Filter is then the entry
// filter), else the kind whose search binds Conditions.
type KindFilter struct {
	Kind       string
	Conditions []KindCondition
}

// KindFilter reads the filter for the kind it asks about. A kind's search binds each key as one
// parameter, so its keys stand side by side - a key once, its values or'ed with a comma, no group
// across keys - and fields of two kinds in one query would ask for nothing.
func (q Query) KindFilter(path string) (KindFilter, error) {
	if q.Filter == nil {
		return KindFilter{}, nil
	}
	kinds := map[string]bool{}
	collectObjects(q.Filter, kinds)
	if len(kinds) > 1 {
		names := make([]string, 0, len(kinds))
		for kind := range kinds {
			names = append(names, kind)
		}
		slices.Sort(names)
		return KindFilter{}, fieldError(path, "query.kinds_mixed", map[string]string{"kinds": strings.Join(names, ", ")})
	}
	if kinds[ObjectEntry] {
		return KindFilter{Kind: ObjectEntry}, nil
	}
	var kind string
	for k := range kinds {
		kind = k
	}
	units := []any{q.Filter}
	if node, _ := q.Filter.(map[string]any); node["op"] == string(OpAnd) {
		units, _ = node["nodes"].([]any)
	}
	out := KindFilter{Kind: kind}
	for _, unit := range units {
		condition, err := kindCondition(unit, path)
		if err != nil {
			return KindFilter{}, err
		}
		for _, held := range out.Conditions {
			if held.Field == condition.Field {
				return KindFilter{}, fieldError(path, "query.key_twice", map[string]string{"field": condition.Field})
			}
		}
		out.Conditions = append(out.Conditions, condition)
	}
	return out, nil
}

// collectObjects notes the kind of every leaf.
func collectObjects(raw any, kinds map[string]bool) {
	node, _ := raw.(map[string]any)
	if nodes, combines := node["nodes"].([]any); combines {
		for _, child := range nodes {
			collectObjects(child, kinds)
		}
		return
	}
	name, _ := node["field"].(string)
	kinds[objectOf(name)] = true
}

// kindCondition reads one operand of the top AND: a leaf, an OR of one field's leaves (`a, none`),
// or either negated.
func kindCondition(raw any, path string) (KindCondition, error) {
	node, _ := raw.(map[string]any)
	op, _ := node["op"].(string)
	nodes, _ := node["nodes"].([]any)
	switch Operator(op) {
	case OpNot:
		if len(nodes) != 1 {
			return KindCondition{}, fieldError(path, "query.kind_filter_grouped", nil)
		}
		inner, err := kindCondition(nodes[0], path)
		inner.Negated = !inner.Negated
		return inner, err
	case OpOr:
		var out KindCondition
		for i, child := range nodes {
			part, err := kindLeaf(child, path)
			if err != nil {
				return KindCondition{}, err
			}
			if part.Negated || (i > 0 && part.Field != out.Field) {
				return KindCondition{}, fieldError(path, "query.kind_filter_grouped", nil)
			}
			out.Field, out.None = part.Field, out.None || part.None
			out.Values = append(out.Values, part.Values...)
		}
		return out, nil
	case OpAnd:
		return KindCondition{}, fieldError(path, "query.kind_filter_grouped", nil)
	}
	return kindLeaf(raw, path)
}

// kindLeaf reads one comparison against the kind's catalogue, its values typed as a filter
// document's are.
func kindLeaf(raw any, path string) (KindCondition, error) {
	node, _ := raw.(map[string]any)
	if _, combines := node["nodes"]; combines {
		return KindCondition{}, fieldError(path, "query.kind_filter_grouped", nil)
	}
	name, _ := node["field"].(string)
	target, _ := kindField(name)
	op, _ := node["op"].(string)
	if !target.Permits(Operator(op)) {
		return KindCondition{}, fieldError(path, "query.operator_unsupported", map[string]string{"field": name, "operator": op})
	}
	if Operator(op) == OpIsNull {
		return KindCondition{Field: name, None: true}, nil
	}
	values, err := parseValues(node, target, Operator(op), path)
	if err != nil {
		return KindCondition{}, err
	}
	out := KindCondition{Field: name, Negated: Operator(op) == OpNeq || Operator(op) == OpNotIn}
	for _, value := range values {
		switch {
		case value.IsPlaceholder() && value.Placeholder.Kind == PlaceholderMe:
			out.Values = append(out.Values, "@me")
		case value.IsPlaceholder():
			return KindCondition{}, fieldError(path, "query.value_unknown", map[string]string{"field": name})
		case value.Kind == KindID:
			out.Values = append(out.Values, value.ID.String())
		case value.Kind == KindBool:
			out.Values = append(out.Values, strconv.FormatBool(value.Bool))
		default:
			out.Values = append(out.Values, value.Text)
		}
	}
	return out, nil
}
