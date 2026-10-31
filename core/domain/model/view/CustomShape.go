// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"net/netip"
	"slices"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CustomColumn is the typed column a custom field's value is compared, sorted and grouped in
// (F-2): the side table's `num`, `ts`, `text` or `ref`, or none for a kind that is never compared.
type CustomColumn string

const (
	CustomNumber  CustomColumn = "num"
	CustomInstant CustomColumn = "ts"
	CustomText    CustomColumn = "text"
	CustomRef     CustomColumn = "ref"
	// CustomAddress is an IP_ADDRESS's `addr`, ordered as addresses rather than as text.
	CustomAddress CustomColumn = "addr"
	// CustomProse is formatted text, which has no column at all.
	CustomProse CustomColumn = "prose"
)

// CustomShape is what the definitions under one key in a query's scope say about its values. The
// grammar accepts a custom field by its shape alone; the shape is the definitions' answer, read by
// the use case before the query compiles (Spec.Shaped).
type CustomShape struct {
	// Resolved says the use case asked. A shape the compiler meets unresolved is a defect.
	Resolved bool
	// Defined says a live definition stands behind the key. An undefined key compares as having
	// no value on any entry - the honest answer for a key nothing defines, and the one an equality
	// filter on it already gives.
	Defined bool
	// Ambiguous says the scope holds the key under kinds that compare in different columns, so no
	// range, sort or grouping on it has one meaning.
	Ambiguous bool
	Column    CustomColumn
	// Set says one entry may hold several values (MULTI_SELECT, USER_SET, LABEL).
	Set bool
	// Moment says an instant column holds moments (DATE_TIME) rather than days (DATE): a range's
	// bound is read as a moment, and a day and a moment under one key are ambiguous.
	Moment bool
}

// ShapeOf answers the shape of the kinds the definitions under one key have in a scope.
func ShapeOf(kinds []work.CustomFieldKind) CustomShape {
	if len(kinds) == 0 {
		return CustomShape{Resolved: true}
	}
	shape := shapeOfKind(kinds[0])
	for _, kind := range kinds[1:] {
		other := shapeOfKind(kind)
		if other.Column != shape.Column || other.Set != shape.Set || other.Moment != shape.Moment {
			return CustomShape{Resolved: true, Defined: true, Ambiguous: true}
		}
	}
	return shape
}

func shapeOfKind(kind work.CustomFieldKind) CustomShape {
	shape := CustomShape{Resolved: true, Defined: true, Set: kind.IsSet()}
	switch kind {
	case work.CustomFieldNumber, work.CustomFieldMoney, work.CustomFieldDuration, work.CustomFieldBool,
		work.CustomFieldCalculated:
		shape.Column = CustomNumber
	case work.CustomFieldDate:
		shape.Column = CustomInstant
	case work.CustomFieldDateTime:
		shape.Column, shape.Moment = CustomInstant, true
	case work.CustomFieldIPAddress:
		shape.Column = CustomAddress
	case work.CustomFieldUser, work.CustomFieldUserSet, work.CustomFieldReference, work.CustomFieldGroup,
		work.CustomFieldFile:
		shape.Column = CustomRef
	case work.CustomFieldRichText:
		shape.Column = CustomProse
	default:
		shape.Column = CustomText
	}
	return shape
}

// ranges reports whether a range operator means something over the shape.
func (s CustomShape) ranges() bool {
	return !s.Defined ||
		(!s.Ambiguous && slices.Contains([]CustomColumn{CustomNumber, CustomInstant, CustomText, CustomAddress}, s.Column))
}

// sorts reports whether entries can be ordered by the shape's one value.
func (s CustomShape) sorts() bool {
	return !s.Defined ||
		(!s.Ambiguous && !s.Set &&
			slices.Contains([]CustomColumn{CustomNumber, CustomInstant, CustomText, CustomAddress}, s.Column))
}

// groups reports whether entries can be put in one group per value of the shape.
func (s CustomShape) groups() bool {
	return !s.Defined || (!s.Ambiguous && !s.Set && s.Column != CustomProse)
}

// isRange reports whether the operator orders values rather than matching them.
func isRange(op Operator) bool {
	return op == OpLt || op == OpLte || op == OpGt || op == OpGte || op == OpBetween
}

// CustomKeys returns the keys of every custom field the spec names, each once, in the order met.
func (s Spec) CustomKeys() []string {
	var keys []string
	add := func(field Field) {
		if key, ok := CustomField(field.Name); ok && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	if s.Filter != nil {
		walkLeaves(*s.Filter, func(node Node) { add(node.Field) })
	}
	for _, term := range s.Sort {
		add(term.Field)
	}
	add(s.GroupBy.Field)
	return keys
}

func walkLeaves(node Node, visit func(Node)) {
	if node.IsLeaf() {
		visit(node)
		return
	}
	for _, child := range node.Nodes {
		walkLeaves(child, visit)
	}
}

// Shaped puts the definitions' answer on every custom field the spec names, and refuses what the
// answer makes meaningless: a range over a field with no order, a sort or a grouping by a field
// that holds several values or prose, and any of the three over a key whose kinds disagree. A
// range over a date field reads its values as days, which is the one conversion the grammar could
// not make before it knew the kind.
func (s Spec) Shaped(shapes map[string]CustomShape) (Spec, error) {
	shapeFor := func(field Field) (Field, bool) {
		key, ok := CustomField(field.Name)
		if !ok {
			return field, false
		}
		shape, known := shapes[key]
		if !known {
			shape = CustomShape{Resolved: true}
		}
		field.Custom = shape
		return field, true
	}

	if s.Filter != nil {
		filter, err := shapedNode(*s.Filter, shapeFor, "/filter")
		if err != nil {
			return Spec{}, err
		}
		s.Filter = &filter
	}

	sort := slices.Clone(s.Sort)
	for index, term := range sort {
		field, custom := shapeFor(term.Field)
		if !custom {
			continue
		}
		if !field.Custom.sorts() {
			return Spec{}, fieldError("/sort/"+strconv.Itoa(index)+"/field", "query.field_not_sortable",
				map[string]string{"field": field.Name})
		}
		sort[index].Field = field
	}
	s.Sort = sort

	if field, custom := shapeFor(s.GroupBy.Field); custom {
		if !field.Custom.groups() {
			return Spec{}, fieldError("/group_by/field", "query.field_not_groupable",
				map[string]string{"field": field.Name})
		}
		s.GroupBy.Field = field
	}
	return s, nil
}

func shapedNode(node Node, shapeFor func(Field) (Field, bool), path string) (Node, error) {
	if !node.IsLeaf() {
		children := make([]Node, 0, len(node.Nodes))
		for index, child := range node.Nodes {
			shaped, err := shapedNode(child, shapeFor, path+"/nodes/"+strconv.Itoa(index))
			if err != nil {
				return Node{}, err
			}
			children = append(children, shaped)
		}
		node.Nodes = children
		return node, nil
	}

	field, custom := shapeFor(node.Field)
	if !custom {
		return node, nil
	}
	node.Field = field
	if node.Op == OpContainsAny || node.Op == OpContainsAll || node.Op == OpStartsWith {
		return typedOperator(node, path)
	}
	if !isRange(node.Op) {
		return canonicalAddresses(node), nil
	}
	if !field.Custom.ranges() {
		return Node{}, fieldError(path+"/op", "query.operator_unsupported", map[string]string{
			"field": field.Name, "operator": string(node.Op),
		})
	}
	values := make([]Value, 0, len(node.Values))
	for index, value := range node.Values {
		typed, err := rangeValue(field.Custom, value, path+"/value"+valueIndex(node, index))
		if err != nil {
			return Node{}, err
		}
		values = append(values, typed)
	}
	node.Values = values
	return node, nil
}

// typedOperator holds a set or prefix operator to the shape it means something over: a set for
// CONTAINS_ANY and CONTAINS_ALL, text for STARTS_WITH, whose value is text too. An undefined key
// passes, and matches nothing, as every other operator on it does.
func typedOperator(node Node, path string) (Node, error) {
	shape := node.Field.Custom
	fits := !shape.Defined
	if shape.Defined && !shape.Ambiguous {
		if node.Op == OpStartsWith {
			fits = !shape.Set && (shape.Column == CustomText || shape.Column == CustomAddress)
		} else {
			fits = shape.Set
		}
	}
	if !fits {
		return Node{}, fieldError(path+"/op", "query.operator_unsupported", map[string]string{
			"field": node.Field.Name, "operator": string(node.Op),
		})
	}
	if node.Op == OpStartsWith && node.Values[0].Kind != KindString {
		return Node{}, fieldError(path+"/value", "query.value_type_invalid", nil)
	}
	return node, nil
}

// rangeValue holds a bound to the column it is compared in.
func rangeValue(shape CustomShape, value Value, path string) (Value, error) {
	if !shape.Defined {
		return value, nil
	}
	switch shape.Column {
	case CustomNumber:
		if value.Kind != KindNumber {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
	case CustomInstant:
		if value.Kind != KindString {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
		if shape.Moment {
			moment, err := time.Parse(time.RFC3339Nano, value.Text)
			if err != nil {
				return Value{}, fieldError(path, "query.value_type_invalid", nil)
			}
			return Value{Kind: KindTimestamp, Time: moment.UTC()}, nil
		}
		day, err := time.Parse(time.DateOnly, value.Text)
		if err != nil {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
		return Value{Kind: KindTimestamp, Time: day}, nil
	case CustomText:
		if value.Kind != KindString {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
	case CustomAddress:
		address, err := netip.ParseAddr(value.Text)
		if value.Kind != KindString || err != nil || address.Zone() != "" {
			return Value{}, fieldError(path, "query.value_type_invalid", nil)
		}
		return Value{Kind: KindString, Text: address.String()}, nil
	}
	return value, nil
}

// canonicalAddresses spells an address an equality compares the way the field stores it, so
// `2001:DB8::1` finds what was written `2001:db8::1`. A value that is no address stays as it was
// and matches nothing.
func canonicalAddresses(node Node) Node {
	if !node.Field.Custom.Defined || node.Field.Custom.Column != CustomAddress {
		return node
	}
	values := slices.Clone(node.Values)
	for index, value := range values {
		if address, err := netip.ParseAddr(value.Text); value.Kind == KindString && err == nil {
			values[index].Text = address.String()
		}
	}
	node.Values = values
	return node
}

func valueIndex(node Node, index int) string {
	if node.Op == OpBetween {
		return "/" + strconv.Itoa(index)
	}
	return ""
}
