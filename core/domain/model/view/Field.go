// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package view holds the query language every view is drawn from: the filter grammar of
// api-guidelines.md §3, what may be asked of which field, and the bounds a query has to stay
// inside.
//
// It is a domain package and not a query builder. Nothing here knows a column, a table or a
// dialect - it decides what a query *means* and refuses what it does not. Turning a validated
// query into SQL is an adapter's job, and the split is what ADR-0026 rests on: the adapter's text
// is chosen by the types below, so no byte of a request can reach it.
package view

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Kind is what a value for a field looks like.
//
// Its own vocabulary rather than the use case catalogue's Kind: that one describes the arguments
// of a call, this one the comparable shape of a stored field - `text` is what a full-text operator
// reads and `id_set` a relation an entry has several of, and neither is a thing an argument can be.
type Kind string

const (
	KindString    Kind = "string"
	KindText      Kind = "text"
	KindID        Kind = "id"
	KindBool      Kind = "boolean"
	KindInt       Kind = "integer"
	KindTimestamp Kind = "timestamp"
	KindEnum      Kind = "enum"
	KindIDSet     Kind = "id_set"
	// KindCustom is a value whose shape a definition decides rather than this catalogue (C-07).
	//
	// The custom fields are the one family this package cannot enumerate: their keys are data a
	// tenant wrote, and what a key holds is the definition's answer. So the grammar accepts the
	// family by shape - `custom_fields.<key>`, with the key matching the same rule the definition
	// enforces - and compares as text, which is what every kind has a spelling in.
	KindCustom Kind = "custom"
	// KindNumber is the shape a *value* takes when it is a JSON number and no column pins its
	// type: a custom field's. Never a field's kind - `depth` is a KindInt column - so it appears
	// only on Value, where it says which JSON scalar the comparison is against.
	KindNumber Kind = "number"
)

// Field is one thing a query may ask about.
//
// The catalogue below is the whole of what this installation accepts, and `/meta/capabilities`
// answers it verbatim: a client builds its filter editor from that list, so a field that is here
// and unusable, or usable and not here, is a client rendering something the server refuses
// (api-guidelines.md §3).
type Field struct {
	Name string
	Kind Kind
	// Operators are the comparisons this field permits, and the list is exhaustive: an operator
	// that is not in it is refused for this field even when the operator itself exists. A field
	// with none is one that may only be sorted or grouped by.
	Operators []Operator
	// Values are the permitted values of an enum field, empty for every other kind.
	Values []string
	// Nullable says whether the field can be absent - which is what makes IS_NULL mean something,
	// and what decides whether a sort has to place the nulls.
	Nullable  bool
	Sortable  bool
	Groupable bool
}

// Permits reports whether the field accepts the operator.
func (f Field) Permits(op Operator) bool { return slices.Contains(f.Operators, op) }

// The fields this version serves.
//
// Deliberately not every column `work_item` has. `due_at`, `start_at` and the custom fields have
// had columns and indexes since the first migration, and no use case writes any of them yet - a
// filter on them would match nothing, and a client could not tell that from a collection in which
// nothing is due. They are refused by name until the milestone that fills them, which is the honest
// answer and the one a client can act on. The assignee and the members left this sentence with
// C-01, which is the task that gave them use cases.
const (
	FieldType        = "type"
	FieldParentID    = "parent_id"
	FieldCollection  = "collection_id"
	FieldBucketID    = "bucket_id"
	FieldIsCompleted = "is_completed"
	FieldTitle       = "title"
	FieldNotes       = "notes"
	FieldDepth       = "depth"
	FieldOrderKey    = "order_key"
	FieldCreatedBy   = "created_by"
	FieldCreatedAt   = "created_at"
	FieldUpdatedAt   = "updated_at"
	FieldCompletedAt = "completed_at"
	FieldArchivedAt  = "archived_at"
	FieldLabels      = "labels"
	FieldAssigneeID  = "assignee_id"
	FieldMembers     = "members"
	// FieldText is the one field with no column of its own: the full-text index over an entry's
	// title and notes, which is what MATCHES reads. A virtual name rather than `title` or `notes`,
	// because the vector covers both and a client that searched `title` and matched a word in the
	// notes would be reading a lie about which field it asked for.
	FieldText = "text"

	// CustomFieldPrefix names the family (api-guidelines.md §3). The one field name in the
	// grammar that carries a value: everything after the dot is a key, which is why it is a
	// parameter in the compiled statement and never part of the text (ADR-0026, T-06).
	CustomFieldPrefix = "custom_fields."
	// MaxCustomFieldKeyLength mirrors the definition's bound (work.MaxCustomFieldKeyLength).
	// Repeated rather than imported, because a longer key here would be a name the grammar accepts
	// and no definition can ever have.
	MaxCustomFieldKeyLength = 50
)

// customFieldOperators are what may be asked of a custom field.
//
// Narrower than a typed column's, and deliberately: the kind is not known when the filter is
// validated - it is the definition's, which is data - so only the comparisons that mean the same
// thing for every kind are offered. Ordering comparisons are absent for that reason: `LT` over a
// jsonb value would compare a NUMBER as text on one entry and as a number on the next.
var customFieldOperators = []Operator{
	OpEq, OpNeq, OpIn, OpNotIn, OpIsNull, OpContains,
}

// CustomField reports whether the name belongs to the family, and answers with the key.
//
// The key is validated here rather than trusted, and against the same rule a definition enforces:
// a name the grammar accepted and no definition could carry would be a filter that can only ever
// match nothing, and the compiler would be binding a parameter nobody wrote.
func CustomField(name string) (string, bool) {
	key, found := strings.CutPrefix(name, CustomFieldPrefix)
	if !found || key == "" || len(key) > MaxCustomFieldKeyLength {
		return "", false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_'):
		default:
			return "", false
		}
	}
	return key, true
}

// customFieldEntry is the synthetic field one `custom_fields.<key>` name resolves to.
func customFieldEntry(name string) Field {
	return Field{
		Name: name, Kind: KindCustom, Operators: customFieldOperators, Nullable: true,
	}
}

// catalogue is the field list, in the order `/meta/capabilities` reports it: the structural fields
// first, then the content, then the timestamps, then the relations.
var catalogue = []Field{
	{
		Name: FieldType, Kind: KindEnum,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpNotIn},
		Values: []string{
			string(work.ItemTask), string(work.ItemWorkPackage), string(work.ItemActivity),
		},
		Sortable: true, Groupable: true,
	},
	{
		Name: FieldParentID, Kind: KindID,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpNotIn, OpIsNull},
		Nullable:  true,
	},
	{
		Name: FieldCollection, Kind: KindID,
		Operators: []Operator{OpEq, OpIn},
		Groupable: true,
	},
	{
		Name: FieldBucketID, Kind: KindID,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpNotIn, OpIsNull},
		Nullable:  true, Groupable: true,
	},
	{
		Name: FieldIsCompleted, Kind: KindBool,
		Operators: []Operator{OpEq, OpNeq},
		Sortable:  true, Groupable: true,
	},
	{
		Name: FieldTitle, Kind: KindString,
		Operators: []Operator{OpEq, OpNeq, OpContains, OpStartsWith},
		Sortable:  true,
	},
	{
		Name: FieldNotes, Kind: KindText,
		Operators: []Operator{OpContains, OpIsNull},
		Nullable:  true,
	},
	{
		Name: FieldDepth, Kind: KindInt,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpLt, OpLte, OpGt, OpGte, OpBetween},
		Sortable:  true,
	},
	{
		// Sort only. A rank key is a fractional index whose value means nothing outside its own
		// level (core/domain/service/Ordering.go), so comparing one against a value a client wrote
		// down is a question with no answer - but ordering by it is the manual order every board
		// and list is drawn in, and the default sort of this whole endpoint.
		Name: FieldOrderKey, Kind: KindString,
		Sortable: true,
	},
	{
		Name: FieldCreatedBy, Kind: KindID,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpNotIn},
		Groupable: true,
	},
	{
		Name: FieldCreatedAt, Kind: KindTimestamp,
		Operators: []Operator{OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte, OpBetween},
		Sortable:  true,
	},
	{
		Name: FieldUpdatedAt, Kind: KindTimestamp,
		Operators: []Operator{OpEq, OpNeq, OpLt, OpLte, OpGt, OpGte, OpBetween},
		Sortable:  true,
	},
	{
		Name: FieldCompletedAt, Kind: KindTimestamp,
		Operators: []Operator{OpLt, OpLte, OpGt, OpGte, OpBetween, OpIsNull},
		Nullable:  true, Sortable: true,
	},
	{
		Name: FieldArchivedAt, Kind: KindTimestamp,
		Operators: []Operator{OpLt, OpLte, OpGt, OpGte, OpBetween, OpIsNull},
		Nullable:  true, Sortable: true,
	},
	{
		// A set, so the set operators and not equality: an entry carries several labels, and
		// `labels EQ x` would have to mean either "carries x" or "carries exactly x" - two
		// different questions behind one spelling.
		Name: FieldLabels, Kind: KindIDSet,
		Operators: []Operator{OpContains, OpContainsAny, OpContainsAll},
	},
	{
		// A scalar, so equality rather than the set operators: an entry has one assignee, and
		// `assignee_id EQ x` has exactly one meaning. IS_NULL is what "nobody is on this" is asked
		// as, and grouping by it is the board every "by person" view is drawn from.
		Name: FieldAssigneeID, Kind: KindID,
		Operators: []Operator{OpEq, OpNeq, OpIn, OpNotIn, OpIsNull},
		Nullable:  true, Groupable: true,
	},
	{
		// A set, like the labels and for the same reason: an entry carries several members, and
		// `members EQ x` would have to mean either "carries x" or "carries exactly x" - two
		// different questions behind one spelling.
		Name: FieldMembers, Kind: KindIDSet,
		Operators: []Operator{OpContains, OpContainsAny, OpContainsAll},
	},
	{
		Name: FieldText, Kind: KindText,
		Operators: []Operator{OpMatches},
	},
}

// The custom fields are deliberately not in the catalogue.
//
// `/meta/capabilities` publishes it verbatim, and everything in it has to be a name a filter may
// use - a client builds its editor from the list, so an entry that is published and refused is a
// client rendering something the server rejects (api-guidelines.md §3). A family cannot satisfy
// that: `custom_fields.*` is a shape rather than a field, and the keys are data. Where a client
// learns which keys exist is `/custom-fields`, which answers the definitions themselves - with
// their kinds and their options, which is more than the manifest could have said anyway.

// Fields returns the catalogue, in its own order. A copy, because the manifest hands it out.
func Fields() []Field { return slices.Clone(catalogue) }

// FieldByName looks a field up. The comparison is exact: a name is an identifier of the contract,
// and accepting `Title` for `title` would make the manifest a suggestion.
func FieldByName(name string) (Field, bool) {
	for _, field := range catalogue {
		if field.Name == name {
			return field, true
		}
	}
	return Field{}, false
}

// field resolves a name or explains why it is not one, pointing at the place in the request that
// named it.
func field(name, path string) (Field, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Field{}, fieldError(path, "query.field_required", nil)
	}
	if key, isCustom := CustomField(name); isCustom {
		// A key rather than a column, and the whole reason this branch exists: the family cannot
		// be enumerated, so it is recognised by shape. What the key may hold is the definition's
		// answer at write time, not this grammar's - a filter on a key nothing defines is a filter
		// that matches nothing, which is the honest result rather than a refusal.
		_ = key
		return customFieldEntry(name), nil
	}
	found, known := FieldByName(name)
	if !known {
		// The name is echoed back. It came from the request and is a field name rather than
		// content, so it carries nothing of anybody's data - and a client that mistyped one has to
		// see which (ai-first.md §1.2).
		return Field{}, fieldError(path, "query.field_unknown", map[string]string{"field": name})
	}
	return found, nil
}

// fieldError is the one shape every refusal in this package takes: a validation error carrying the
// JSON pointer of the part of the query that is wrong, so a client can mark the input rather than
// the request.
func fieldError(path, code string, params map[string]string) error {
	err := shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code, Params: params})
	if params != nil {
		err = err.WithParams(params)
	}
	return err
}
