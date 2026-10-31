// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"math"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The typed side of a custom field (F-2): ranges, sorts and groupings read `entry_field_value`,
// which migration 0211's trigger keeps beside the jsonb document. The key is bound as a
// parameter, as everywhere else in this package, and the column is chosen by the shape the use
// case put on the field - never by a value (ADR-0026).

// customTextLength bounds a text value the way the side table does (migration 0211), so that a
// cursor's key and a group's key match the stored one.
const customTextLength = 256

// customDefinitionJoin narrows the side table to the live definition under one key, in the entry's
// own collection or workspace-wide - the read projection's visibility, by identity.
const customDefinitionJoin = `entry_field_value efv JOIN custom_field_definition cfd ` +
	`ON cfd.tenant_id = efv.tenant_id AND cfd.id = efv.definition_id ` +
	`WHERE efv.tenant_id = wi.tenant_id AND efv.entry_id = wi.id ` +
	`AND cfd.deleted_at IS NULL AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL) ` +
	`AND cfd.key = `

func customColumn(shape view.CustomShape) (string, bool) {
	switch shape.Column {
	case view.CustomNumber:
		return `efv.num`, true
	case view.CustomInstant:
		return `efv.ts`, true
	case view.CustomText:
		return `efv.text`, true
	case view.CustomRef:
		return `efv.ref`, true
	case view.CustomAddress:
		return `efv.addr`, true
	}
	return "", false
}

func customCast(shape view.CustomShape) string {
	if !shape.Defined {
		return `::text`
	}
	switch shape.Column {
	case view.CustomNumber:
		return `::numeric`
	case view.CustomInstant:
		return `::timestamptz`
	case view.CustomRef:
		return `::uuid`
	case view.CustomAddress:
		return `::inet`
	}
	return `::text`
}

var errShapeUnresolved = shared.ErrInternal.WithDetail("query.custom_field_unshaped")

// customValue writes the one typed value a custom field holds on the row, or a typed NULL where
// nothing defines the key.
func (b *builder) customValue(field view.Field) {
	key, _ := view.CustomField(field.Name)
	shape := field.Custom
	if !shape.Resolved {
		b.fail(errShapeUnresolved)
		return
	}
	column, ok := customColumn(shape)
	if !shape.Defined || !ok || shape.Set || shape.Ambiguous {
		if shape.Defined {
			b.fail(errFieldNotCompilable(field))
			return
		}
		b.write(`NULL::text`)
		return
	}
	b.write(`(SELECT `, column, ` FROM `, customDefinitionJoin)
	b.param(key)
	b.write(`::text AND efv.ordinal = 0)`)
}

// customGroupKey writes the value as the text a group is keyed by - the spelling customKeyOf gives
// the same value in Go.
func (b *builder) customGroupKey(field view.Field) {
	if field.Custom.Defined && field.Custom.Column == view.CustomInstant {
		b.write(`((`)
		b.customValue(field)
		b.write(`) AT TIME ZONE 'UTC')::date::text`)
		return
	}
	if field.Custom.Defined && field.Custom.Column == view.CustomAddress {
		// host() is the address without its mask, the spelling netip gives in customKeyOf.
		b.write(`host(`)
		b.customValue(field)
		b.write(`)`)
		return
	}
	b.write(`(`)
	b.customValue(field)
	b.write(`)::text`)
}

// customRange writes an ordering comparison: does any value the entry holds under the key fall in
// the range. Any rather than the first, so that a range over a set means "holds one in range".
func (b *builder) customRange(node view.Node, key string) {
	shape := node.Field.Custom
	if !shape.Resolved {
		b.fail(errShapeUnresolved)
		return
	}
	column, ok := customColumn(shape)
	if !shape.Defined {
		// Nothing defines the key, so no entry holds a value in any range.
		b.write(`false`)
		return
	}
	if !ok || shape.Ambiguous || shape.Column == view.CustomRef {
		b.fail(errOperatorNotCompilable(node.Op))
		return
	}

	b.write(`EXISTS (SELECT 1 FROM `, customDefinitionJoin)
	b.param(key)
	b.write(`::text AND `, column)
	switch node.Op {
	case view.OpLt:
		b.write(` < `)
	case view.OpLte:
		b.write(` <= `)
	case view.OpGt:
		b.write(` > `)
	case view.OpGte:
		b.write(` >= `)
	case view.OpBetween:
		b.write(` BETWEEN `)
		b.customBound(shape, node.Values[0])
		b.write(` AND `)
		b.customBound(shape, node.Values[1])
		b.write(`)`)
		return
	default:
		b.fail(errOperatorNotCompilable(node.Op))
		return
	}
	b.customBound(shape, node.Values[0])
	b.write(`)`)
}

func (b *builder) customBound(shape view.CustomShape, value view.Value) {
	switch shape.Column {
	case view.CustomNumber:
		b.param(value.Text)
		b.write(`::numeric`)
	case view.CustomInstant:
		b.param(value.Time)
		b.write(`::timestamptz`)
	case view.CustomAddress:
		b.param(value.Text)
		b.write(`::inet`)
	default:
		b.text(value)
	}
}

// customBoundary writes a cursor's key for a custom sort term.
func (b *builder) customBoundary(field view.Field, raw string) {
	switch {
	case !field.Custom.Defined:
		b.fail(errCursorInvalid)
	case field.Custom.Column == view.CustomNumber:
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(raw)
		b.write(`::numeric`)
	case field.Custom.Column == view.CustomInstant:
		moment, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(moment.UTC())
		b.write(`::timestamptz`)
	case field.Custom.Column == view.CustomAddress:
		address, err := netip.ParseAddr(raw)
		if err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(address.String())
		b.write(`::inet`)
	default:
		b.param(raw)
		b.write(`::text`)
	}
}

// customKeyOf renders the value an entry holds under a custom field the way the side table holds
// it: a number in its shortest spelling, a yes/no as 1 or 0, a date as its UTC midnight, text cut
// to the stored length, an account or entry as its canonical identifier. False for no value.
func customKeyOf(field view.Field, item work.WorkItem, forCursor bool) (string, bool) {
	key, _ := view.CustomField(field.Name)
	value, present := item.CustomFields[key]
	if !present || value == nil || !field.Custom.Defined {
		return "", false
	}
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "", false
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case bool:
		if typed {
			return "1", true
		}
		return "0", true
	case string:
		switch field.Custom.Column {
		case view.CustomInstant:
			if field.Custom.Moment {
				moment, err := time.Parse(time.RFC3339, typed)
				if err != nil {
					return "", false
				}
				if forCursor {
					return moment.UTC().Format(time.RFC3339Nano), true
				}
				return moment.UTC().Format(time.DateOnly), true
			}
			day, err := time.Parse(time.DateOnly, typed)
			if err != nil {
				return "", false
			}
			if forCursor {
				return day.UTC().Format(time.RFC3339Nano), true
			}
			return day.Format(time.DateOnly), true
		case view.CustomRef:
			id, err := shared.ParseID(typed)
			if err != nil {
				return "", false
			}
			return id.String(), true
		case view.CustomAddress:
			address, err := netip.ParseAddr(typed)
			if err != nil {
				return "", false
			}
			return address.String(), true
		}
		return truncateRunes(typed, customTextLength), true
	}
	return "", false
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	var cut strings.Builder
	for index, r := range []rune(text) {
		if index == limit {
			break
		}
		cut.WriteRune(r)
	}
	return cut.String()
}

// GroupKey is the key a grouped query's row belongs to under a custom field: the spelling Count
// keys the same group by. False for an entry with no value.
func GroupKey(field view.Field, item work.WorkItem) (string, bool) {
	return customKeyOf(field, item, false)
}

// sortAlias names the column a grouped query's inner select carries a custom sort term in, so the
// outer select can order by it without the row alias it does not have.
var sortAlias = [view.MaxSortTerms]string{`sk0`, `sk1`, `sk2`, `sk3`}

// groupAlias is the same for the grouping value.
const groupAlias = `gk`
