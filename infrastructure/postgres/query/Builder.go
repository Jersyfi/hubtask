// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// builder accumulates a statement and the values it binds.
//
// The two halves are written together and can therefore not disagree: nothing appends text without
// going through write, which takes constants, and nothing binds a value without going through
// param, which writes its own placeholder. statement() then checks that the two ended up the same
// length, so a fragment that forgot its argument fails here rather than shifting every later
// parameter by one and asking the database a different question (ADR-0026).
type builder struct {
	sql  strings.Builder
	args []any
	err  error
}

// write appends constant text. Every argument is a literal of this package or a value returned by
// one of the switches below - which is the whole rule, and the reason `fmt` is not importable here.
func (b *builder) write(fragments ...string) {
	for _, fragment := range fragments {
		b.sql.WriteString(fragment)
	}
}

// param binds a value and writes the placeholder that refers to it. The number is the builder's own
// count, not anything that came from the request.
func (b *builder) param(value any) {
	b.args = append(b.args, value)
	b.sql.WriteByte('$')
	b.sql.WriteString(strconv.Itoa(len(b.args)))
}

// fail records the first thing that went wrong and lets the rest of the walk finish. Half a
// statement is never executed - statement() reports the failure instead - and stopping at the first
// error would mean threading a return value through every writer above.
func (b *builder) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

func (b *builder) statement() (Statement, error) {
	if b.err != nil {
		return Statement{}, b.err
	}

	sql := b.sql.String()
	// The count is exact because no fragment in this package contains a dollar sign for any other
	// purpose: every one of them is a placeholder param wrote.
	if strings.Count(sql, "$") != len(b.args) {
		return Statement{}, shared.ErrInternal.WithDetail("query.parameters_inconsistent")
	}
	return Statement{SQL: sql, Args: b.args}, nil
}

// uuid binds an identifier. Cast rather than converted, so that the parameter travels as text and
// this package needs no driver type - which is what keeps it free of the driver altogether.
func (b *builder) uuid(id shared.ID) {
	b.param(id.String())
	b.write(`::uuid`)
}

// text binds a text value, normalised the way the column it is compared against was written.
//
// The title is stored NFC normalised, in the database (db/queries/Work.sql). A comparison value
// that skipped the normalisation would fail to match a title that differs from it only in how a
// combining accent is encoded - two spellings of the same word, and a search that silently misses.
func (b *builder) text(value view.Value) {
	b.write(`normalize(`)
	b.param(value.Text)
	b.write(`::text, NFC)`)
}

// value writes one comparison value with the type its column expects.
func (b *builder) value(field view.Field, value view.Value) {
	if value.IsPlaceholder() {
		// A placeholder that reached the adapter was never resolved, and `@me` is not a value any
		// column can be compared against. A defect in the use case rather than in the request.
		b.fail(shared.ErrInternal.WithDetail("query.placeholder_unresolved"))
		return
	}

	switch field.Kind {
	case view.KindID, view.KindIDSet:
		b.uuid(value.ID)
	case view.KindEnum:
		b.param(value.Text)
		b.write(enumCast(field))
	case view.KindBool:
		b.param(value.Bool)
		b.write(`::boolean`)
	case view.KindInt:
		b.param(value.Int)
		b.write(`::bigint`)
	case view.KindTimestamp:
		b.param(value.Time)
		b.write(`::timestamptz`)
	default:
		b.text(value)
	}
}

// array binds a list operator's values as one parameter.
//
// One array rather than a placeholder per element: the number of elements comes from the request,
// and a statement whose shape follows the request is one the database has to plan afresh every
// time. With an array the text is the same for one label and for a hundred.
func (b *builder) array(field view.Field, values []view.Value) {
	switch field.Kind {
	case view.KindID, view.KindIDSet:
		b.param(textsOf(values, func(v view.Value) string { return v.ID.String() }))
		b.write(`::uuid[]`)
	case view.KindEnum:
		b.param(textsOf(values, func(v view.Value) string { return v.Text }))
		b.write(enumCast(field), `[]`)
	case view.KindInt:
		numbers := make([]int64, 0, len(values))
		for _, value := range values {
			numbers = append(numbers, value.Int)
		}
		b.param(numbers)
		b.write(`::bigint[]`)
	default:
		b.param(textsOf(values, func(v view.Value) string { return v.Text }))
		b.write(`::text[]`)
	}
}

func textsOf(values []view.Value, of func(view.Value) string) []string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, of(value))
	}
	return texts
}

// column maps a field of the catalogue onto the column that holds it.
//
// The second half of ADR-0026's "two places, and neither takes text from a request": the grammar
// decides that a name is a field, and this decides which column that field is. A name that reaches
// here and is not in this switch is a field somebody added to the catalogue and not to the schema,
// which is a defect and is reported as one.
func column(field view.Field, prefix string) (string, bool) {
	switch field.Name {
	case view.FieldType:
		return prefix + `type`, true
	case view.FieldParentID:
		return prefix + `parent_id`, true
	case view.FieldCollection:
		return prefix + `collection_id`, true
	case view.FieldBucketID:
		return prefix + `bucket_id`, true
	case view.FieldIsCompleted:
		return prefix + `is_completed`, true
	case view.FieldTitle:
		return prefix + `title`, true
	case view.FieldNotes:
		return prefix + `notes`, true
	case view.FieldDepth:
		return prefix + `depth`, true
	case view.FieldOrderKey:
		return prefix + `order_key`, true
	case view.FieldCreatedBy:
		return prefix + `created_by`, true
	case view.FieldCreatedAt:
		return prefix + `created_at`, true
	case view.FieldUpdatedAt:
		return prefix + `updated_at`, true
	case view.FieldCompletedAt:
		return prefix + `completed_at`, true
	case view.FieldArchivedAt:
		return prefix + `archived_at`, true
	}
	return "", false
}

// enumCast is the database type behind an enum field. One field has one today, and the switch is
// what keeps the next one from silently borrowing this one's type.
func enumCast(field view.Field) string {
	if field.Name == view.FieldType {
		return `::item_type`
	}
	return `::text`
}

// collationOf is where a comparison has to leave the database's collation behind.
//
// A rank key is a fractional index whose scheme rests on byte order: `A`..`Z` head the negative
// integers and `a`..`z` the non-negative ones (core/domain/service/Ordering.go). Under a glibc
// `en_US.UTF-8` the two spaces interleave, and the order this query returns would then disagree
// with the order the domain assigns - so the collation is stated, here and in the index that serves
// it (migration 0010), rather than inherited from whatever the database was created with.
func collationOf(field view.Field) string {
	if field.Name == view.FieldOrderKey {
		return ` COLLATE "C"`
	}
	return ""
}

// The two spellings a cursor key can have. A single leading byte, because a boundary of "no value
// at all" is a real boundary - a sort by `completed_at` with the nulls last ends in them - and an
// empty string would be indistinguishable from an empty title.
const (
	keyNull  = "n"
	keyValue = "v"
)

// Key renders one sort field of one row as the text a cursor carries.
//
// Here rather than in the adapter, next to the parsing that reads it back: the two have to agree
// byte for byte, and a boundary that encodes as one thing and decodes as another is a page that
// skips rows rather than an error anybody sees.
func Key(term view.SortTerm, item work.WorkItem) string {
	switch term.Field.Name {
	case view.FieldOrderKey:
		return keyValue + item.OrderKey
	case view.FieldTitle:
		return keyValue + item.Title
	case view.FieldType:
		return keyValue + string(item.Type)
	case view.FieldIsCompleted:
		return keyValue + strconv.FormatBool(item.Completion.IsCompleted)
	case view.FieldDepth:
		return keyValue + strconv.Itoa(item.Depth)
	case view.FieldCreatedAt:
		return timeKey(item.CreatedAt)
	case view.FieldUpdatedAt:
		return timeKey(item.UpdatedAt)
	case view.FieldCompletedAt:
		return optionalTimeKey(item.Completion.CompletedAt)
	case view.FieldArchivedAt:
		return optionalTimeKey(item.ArchivedAt)
	}
	return keyNull
}

// optionalTimeKey is the same for a stamp that may not be there at all - which is what makes the
// null spelling of a key necessary rather than decorative.
func optionalTimeKey(moment *time.Time) string {
	if moment == nil {
		return keyNull
	}
	return timeKey(*moment)
}

func timeKey(moment time.Time) string {
	if moment.IsZero() {
		return keyNull
	}
	// Nanoseconds, so that a boundary is exact at the resolution the column keeps. Two rows written
	// in the same microsecond are separated by the identifier that follows the key.
	return keyValue + moment.UTC().Format(time.RFC3339Nano)
}

// boundaryValue writes the far side of the keyset comparison: the cursor's key for this term, typed
// as its column is.
func (b *builder) boundaryValue(term view.SortTerm, key string) {
	if key == keyNull {
		b.write(`NULL`, nullCast(term.Field))
		return
	}
	raw, ok := strings.CutPrefix(key, keyValue)
	if !ok {
		b.fail(errCursorInvalid)
		return
	}

	switch term.Field.Kind {
	case view.KindBool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(value)
		b.write(`::boolean`)

	case view.KindInt:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(value)
		b.write(`::bigint`)

	case view.KindTimestamp:
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			b.fail(errCursorInvalid)
			return
		}
		b.param(value.UTC())
		b.write(`::timestamptz`)

	case view.KindEnum:
		b.param(raw)
		b.write(enumCast(term.Field))

	default:
		b.param(raw)
		b.write(`::text`)
	}
}

// nullCast types a NULL boundary, because an untyped one in a comparison is something the database
// refuses to plan.
func nullCast(field view.Field) string {
	switch field.Kind {
	case view.KindTimestamp:
		return `::timestamptz`
	case view.KindID, view.KindIDSet:
		return `::uuid`
	case view.KindBool:
		return `::boolean`
	case view.KindInt:
		return `::bigint`
	case view.KindEnum:
		return enumCast(field)
	default:
		return `::text`
	}
}

// errCursorInvalid is the client's answer to a boundary this server cannot read: start the walk
// again. It is the same answer the cursor codec gives for a tampered one, and deliberately so -
// telling the two apart would tell whoever is probing how far their forgery got (security.md §8).
var errCursorInvalid = shared.ErrValidation.
	WithDetail("shared.cursor_invalid").
	WithFields(shared.FieldError{Path: "/page/cursor", Code: "shared.cursor_invalid"})

// The two defects the grammar should have made unreachable. They are internal errors rather than
// refusals: a field in the catalogue with no column, or an operator a field permits and this
// package cannot write, is something a release broke rather than something a client sent.
func errFieldNotCompilable(field view.Field) error {
	return shared.ErrInternal.
		WithDetail("query.field_not_compilable").
		WithParams(map[string]string{"field": field.Name})
}

func errOperatorNotCompilable(op view.Operator) error {
	return shared.ErrInternal.
		WithDetail("query.operator_not_compilable").
		WithParams(map[string]string{"operator": string(op)})
}
