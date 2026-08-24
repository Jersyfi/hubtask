// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CustomFieldKind is what a value for a field looks like. The eight the schema has carried since
// `0001_init` (domain-model.md §3.5).
type CustomFieldKind string

const (
	CustomFieldText        CustomFieldKind = "TEXT"
	CustomFieldNumber      CustomFieldKind = "NUMBER"
	CustomFieldDate        CustomFieldKind = "DATE"
	CustomFieldSelect      CustomFieldKind = "SELECT"
	CustomFieldMultiSelect CustomFieldKind = "MULTI_SELECT"
	CustomFieldBool        CustomFieldKind = "BOOL"
	CustomFieldUser        CustomFieldKind = "USER"
	CustomFieldURL         CustomFieldKind = "URL"
)

var customFieldKinds = [...]CustomFieldKind{
	CustomFieldText, CustomFieldNumber, CustomFieldDate, CustomFieldSelect,
	CustomFieldMultiSelect, CustomFieldBool, CustomFieldUser, CustomFieldURL,
}

// CustomFieldKinds returns every kind, in the order the contract's enum lists them.
func CustomFieldKinds() []CustomFieldKind { return customFieldKinds[:] }

// ParseCustomFieldKind reads a submitted kind.
func ParseCustomFieldKind(value string) (CustomFieldKind, error) {
	kind := CustomFieldKind(value)
	if !slices.Contains(customFieldKinds[:], kind) {
		return "", shared.ErrValidation.
			WithDetail("fields.kind_unknown").
			WithParams(map[string]string{"value": value}).
			WithFields(shared.FieldError{Path: "/kind", Code: "fields.kind_unknown"})
	}
	return kind, nil
}

// TakesOptions reports whether the kind's values are drawn from a list.
func (k CustomFieldKind) TakesOptions() bool {
	return k == CustomFieldSelect || k == CustomFieldMultiSelect
}

// The bounds. Code points rather than bytes, for the reason every length in this model is (I-W7):
// a person counts characters, and a limit in bytes refuses a shorter name in one language than in
// another.
const (
	// MaxCustomFieldKeyLength is the schema's, and the pattern below is the schema's CHECK.
	MaxCustomFieldKeyLength = 50
	// MaxCustomFieldTextLength bounds a TEXT value. Generous, because a custom field is where a
	// person puts what the model did not anticipate - and bounded all the same, because the column
	// is a jsonb map that a query plans over.
	MaxCustomFieldTextLength = 2000
	// MaxCustomFieldOptionLength bounds one option of a SELECT.
	MaxCustomFieldOptionLength = 200
	// MaxCustomFieldOptions bounds how many a SELECT may offer. A list a person picks from, not a
	// table: past this it is a relation and belongs in one.
	MaxCustomFieldOptions = 200
	// MaxCustomFieldSelections bounds one MULTI_SELECT value.
	MaxCustomFieldSelections = 50
	// MaxCustomFieldURLLength bounds a URL value.
	MaxCustomFieldURLLength = 2000
	// MaxCustomFieldsPerItem bounds how many keys one entry may carry. The column is one jsonb
	// document that every read of the entry carries, so its size is the entry's size.
	MaxCustomFieldsPerItem = 100
)

// CustomFieldDefinition is one field a workspace or a collection adds to its entries
// (domain-model.md §3.5, §6).
//
// The values live on the entry, in `work_item.custom_fields`; this says which keys exist, what
// they may hold and which types carry them. Validation is domain code and lives here rather than
// in the database: a `SELECT` value outside its options and a `NUMBER` arriving as a string are
// refusals a client has to be able to act on, and a CHECK constraint cannot say which field was
// wrong.
type CustomFieldDefinition struct {
	ID       shared.ID
	TenantID shared.ID
	// CollectionID is the collection the definition belongs to, zero for the whole workspace. Two
	// scopes and no more: a field every collection defines separately is a field nobody can filter
	// across, and a field only ever defined workspace-wide is one team's vocabulary imposed on
	// every other (domain-model.md §3.5).
	CollectionID shared.ID
	// Key is what the value is stored under. An identifier rather than a label, and fixed once
	// defined: it appears in a `custom_fields.<key>` filter, and a key that could be renamed would
	// orphan every value stored under it.
	Key  string
	Kind CustomFieldKind
	// Options are the permitted values of a SELECT or a MULTI_SELECT, and empty for every other
	// kind. Values rather than labels: what a person sees is the client's to render (rule 8).
	Options []string
	// IsRequired is enforced when the field is written and never retroactively: making a field
	// required does not make the entries that predate it invalid, and a rule that did would make
	// every one of them unsaveable at once.
	IsRequired bool
	// AppliesTo are the item types that carry the field. Never empty - a definition no type
	// carries is one nothing could ever hold.
	AppliesTo []ItemType
	CreatedAt time.Time
	UpdatedAt time.Time
	// DeletedAt is the soft delete. The values stay in the entries and stop being visible:
	// rewriting `custom_fields` across every entry in a collection would be an unbounded write
	// from one request.
	DeletedAt *time.Time
	Version   int
}

// NewCustomFieldInput is what a definition is made of.
type NewCustomFieldInput struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Key          string
	Kind         CustomFieldKind
	Options      []string
	IsRequired   bool
	AppliesTo    []ItemType
	Now          time.Time
}

// NewCustomFieldDefinition builds a definition and checks its invariants.
//
// Uniqueness of the key within its scope is the unique index's decision rather than this type's,
// for the reason NewLabel gives: a check followed by an insert is two statements with a gap
// between them, and two requests arriving in that gap both pass the check.
func NewCustomFieldDefinition(in NewCustomFieldInput) (CustomFieldDefinition, error) {
	key, err := customFieldKey(in.Key)
	if err != nil {
		return CustomFieldDefinition{}, err
	}
	if !slices.Contains(customFieldKinds[:], in.Kind) {
		return CustomFieldDefinition{}, shared.ErrValidation.
			WithDetail("fields.kind_unknown").
			WithParams(map[string]string{"value": string(in.Kind)}).
			WithFields(shared.FieldError{Path: "/kind", Code: "fields.kind_unknown"})
	}
	options, err := customFieldOptions(in.Kind, in.Options)
	if err != nil {
		return CustomFieldDefinition{}, err
	}
	appliesTo, err := customFieldTypes(in.AppliesTo)
	if err != nil {
		return CustomFieldDefinition{}, err
	}
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return CustomFieldDefinition{}, shared.ErrInternal.WithDetail("fields.identity_incomplete")
	}

	return CustomFieldDefinition{
		ID: in.ID, TenantID: in.TenantID, CollectionID: in.CollectionID,
		Key: key, Kind: in.Kind, Options: options,
		IsRequired: in.IsRequired, AppliesTo: appliesTo,
		CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// IsDeleted reports whether the definition is out of use.
func (d CustomFieldDefinition) IsDeleted() bool { return d.DeletedAt != nil }

// IsTenantWide reports whether the definition applies to the whole workspace.
func (d CustomFieldDefinition) IsTenantWide() bool { return d.CollectionID.IsZero() }

// CustomFieldAttributes is what an update may change. A pointer per field, so that "set it to
// nothing" and "do not touch it" stay two different requests.
//
// No key and no kind: a key that moved would orphan every value stored under it, and a kind that
// changed would reinterpret them. Both are a new definition rather than an edit.
type CustomFieldAttributes struct {
	Options    *[]string
	IsRequired *bool
	AppliesTo  *[]ItemType
}

// IsEmpty reports whether the update asks for nothing at all.
func (a CustomFieldAttributes) IsEmpty() bool {
	return a.Options == nil && a.IsRequired == nil && a.AppliesTo == nil
}

// The field names an update reports its changes under.
const (
	FieldOptions    = "options"
	FieldIsRequired = "is_required"
	FieldAppliesTo  = "applies_to"
)

// Updated applies a change and reports which fields moved. Nothing that did not move is in the
// result, and a request that changes nothing returns the definition untouched.
//
// Narrowing the options does not rewrite the entries holding a value that is no longer offered.
// That would be the unbounded write the soft delete exists to avoid, and the value is not wrong -
// it was permitted when it was written. It stops being offered, and the next write of that field
// is refused unless it picks from the new list.
func (d CustomFieldDefinition) Updated(
	attributes CustomFieldAttributes, at time.Time,
) (CustomFieldDefinition, []FieldChange, error) {
	if err := d.EnsureEditable(); err != nil {
		return CustomFieldDefinition{}, nil, err
	}

	var changes []FieldChange

	if attributes.Options != nil {
		options, err := customFieldOptions(d.Kind, *attributes.Options)
		if err != nil {
			return CustomFieldDefinition{}, nil, err
		}
		if !slices.Equal(options, d.Options) {
			changes = append(changes, FieldChange{
				Field: FieldOptions, From: strings.Join(d.Options, ","), To: strings.Join(options, ","),
			})
			d.Options = options
		}
	}

	if attributes.IsRequired != nil && *attributes.IsRequired != d.IsRequired {
		changes = append(changes, FieldChange{
			Field: FieldIsRequired,
			From:  strconv.FormatBool(d.IsRequired), To: strconv.FormatBool(*attributes.IsRequired),
		})
		d.IsRequired = *attributes.IsRequired
	}

	if attributes.AppliesTo != nil {
		appliesTo, err := customFieldTypes(*attributes.AppliesTo)
		if err != nil {
			return CustomFieldDefinition{}, nil, err
		}
		if !slices.Equal(appliesTo, d.AppliesTo) {
			changes = append(changes, FieldChange{
				Field: FieldAppliesTo,
				From:  joinTypes(d.AppliesTo), To: joinTypes(appliesTo),
			})
			d.AppliesTo = appliesTo
		}
	}

	if len(changes) == 0 {
		return d, nil, nil
	}
	d.UpdatedAt = at
	return d, changes, nil
}

// Deleted takes the definition out of use. A soft delete, and idempotent.
func (d CustomFieldDefinition) Deleted(at time.Time) (CustomFieldDefinition, []FieldChange, error) {
	if d.IsDeleted() {
		return d, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: "", To: instant(at)}}
	d.DeletedAt = &at
	d.UpdatedAt = at
	return d, changes, nil
}

// EnsureEditable refuses a definition that is out of use. A conflict rather than a validation
// failure, for the reason Label.EnsureEditable gives.
func (d CustomFieldDefinition) EnsureEditable() error {
	if d.IsDeleted() {
		return shared.ErrConflict.
			WithDetail("fields.deleted").
			WithParams(map[string]string{"field_id": d.ID.String()})
	}
	return nil
}

// Carries reports whether an entry of this type holds the field.
func (d CustomFieldDefinition) Carries(itemType ItemType) bool {
	return slices.Contains(d.AppliesTo, itemType)
}

// customFieldKey keeps a key an identifier: the schema's CHECK, written where a client can be told
// which part of it failed.
func customFieldKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", customFieldError("/key", "fields.key_required", nil)
	}
	if utf8.RuneCountInString(key) > MaxCustomFieldKeyLength {
		return "", customFieldError("/key", "fields.key_too_long",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldKeyLength)})
	}
	if !validCustomFieldKey(key) {
		// The key is echoed back. It is an identifier a client wrote rather than anybody's content,
		// and a client that mistyped one has to see which.
		return "", customFieldError("/key", "fields.key_malformed", map[string]string{"key": key})
	}
	return key, nil
}

// validCustomFieldKey is `^[a-z][a-z0-9_]{0,49}$`, by hand rather than by a regular expression: the
// pattern is four lines of comparison and a compiled expression in a domain package is a dependency
// on a matcher's semantics for no gain.
func validCustomFieldKey(key string) bool {
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_'):
		default:
			return false
		}
	}
	return true
}

// customFieldOptions validates the list a SELECT draws from, and insists the other kinds have none:
// options on a BOOL are a client that misunderstood the field, and storing them would make the
// misunderstanding survive.
func customFieldOptions(kind CustomFieldKind, raw []string) ([]string, error) {
	if !kind.TakesOptions() {
		if len(raw) != 0 {
			return nil, customFieldError("/options", "fields.options_not_applicable",
				map[string]string{"kind": string(kind)})
		}
		return nil, nil
	}
	if len(raw) == 0 {
		return nil, customFieldError("/options", "fields.options_required", nil)
	}
	if len(raw) > MaxCustomFieldOptions {
		return nil, customFieldError("/options", "fields.options_too_many",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldOptions)})
	}

	options := make([]string, 0, len(raw))
	for _, option := range raw {
		value := strings.TrimSpace(option)
		if value == "" {
			return nil, customFieldError("/options", "fields.option_empty", nil)
		}
		if utf8.RuneCountInString(value) > MaxCustomFieldOptionLength {
			return nil, customFieldError("/options", "fields.option_too_long",
				map[string]string{"maximum": strconv.Itoa(MaxCustomFieldOptionLength)})
		}
		if hasControlCharacter(value) {
			return nil, customFieldError("/options", "fields.option_malformed", nil)
		}
		if slices.Contains(options, value) {
			// Two options that read the same are one option a person cannot tell apart, and a
			// stored value would not say which of them was meant.
			return nil, customFieldError("/options", "fields.option_duplicated",
				map[string]string{"option": value})
		}
		options = append(options, value)
	}
	return options, nil
}

// customFieldTypes validates which types carry the field. Whether a type carries custom fields at
// all is the capability matrix's answer and the application's to ask - this only refuses a list
// that is empty or says the same type twice.
func customFieldTypes(raw []ItemType) ([]ItemType, error) {
	if len(raw) == 0 {
		return nil, customFieldError("/applies_to", "fields.applies_to_required", nil)
	}

	types := make([]ItemType, 0, len(raw))
	for _, itemType := range raw {
		if !itemType.Valid() {
			return nil, customFieldError("/applies_to", "items.type_unknown",
				map[string]string{"value": string(itemType)})
		}
		if slices.Contains(types, itemType) {
			continue
		}
		types = append(types, itemType)
	}
	return types, nil
}

func joinTypes(types []ItemType) string {
	names := make([]string, 0, len(types))
	for _, itemType := range types {
		names = append(names, string(itemType))
	}
	return strings.Join(names, ",")
}

// customFieldError is the one shape a refusal in this file takes: the JSON pointer of the part of
// the request that is wrong, so a client can mark the input rather than the whole call.
func customFieldError(path, code string, params map[string]string) error {
	err := shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code, Params: params})
	if params != nil {
		err = err.WithParams(params)
	}
	return err
}

// --- values -----------------------------------------------------------------------------------

// ValidateValue judges one value against the definition and returns it normalised.
//
// The normalisation is the point of returning a value at all: what is stored is what came back
// from here, so a number that arrived as a float and a date that arrived with a time are one shape
// in the column rather than as many shapes as there are clients. A nil value means the key is being
// cleared, which a required field refuses.
//
// The types it accepts are the ones a JSON document produces once decoded - `float64` for every
// number, `string`, `bool`, `[]any` - because that is what reaches this from all three channels.
// A NUMBER arriving as a string is refused rather than parsed: a client that sent "3" meant a
// string, and guessing turns a typo into stored data (domain-model.md §6).
func (d CustomFieldDefinition) ValidateValue(value any) (any, error) {
	if value == nil {
		if d.IsRequired {
			return nil, d.valueError("fields.value_required", nil)
		}
		return nil, nil
	}

	switch d.Kind {
	case CustomFieldText:
		return d.validText(value)
	case CustomFieldNumber:
		return d.validNumber(value)
	case CustomFieldDate:
		return d.validDate(value)
	case CustomFieldSelect:
		return d.validSelect(value)
	case CustomFieldMultiSelect:
		return d.validMultiSelect(value)
	case CustomFieldBool:
		return d.validBool(value)
	case CustomFieldUser:
		return d.validUser(value)
	case CustomFieldURL:
		return d.validURL(value)
	default:
		// A kind no constructor produces. A defect rather than input (security.md §9).
		return nil, shared.ErrInternal.WithDetail("fields.kind_unhandled")
	}
}

func (d CustomFieldDefinition) validText(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		// An empty string is the absence of a value written a second way. One spelling, so that a
		// required field cannot be satisfied by sending "".
		return nil, d.emptyOrCleared()
	}
	if utf8.RuneCountInString(text) > MaxCustomFieldTextLength {
		return nil, d.valueError("fields.value_too_long",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldTextLength)})
	}
	return text, nil
}

func (d CustomFieldDefinition) validNumber(value any) (any, error) {
	number, ok := value.(float64)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		// Neither has a JSON spelling, so neither can be read back out of the column.
		return nil, d.valueError("fields.value_not_finite", nil)
	}
	return number, nil
}

// validDate keeps a date a date: the calendar day, with no time and no zone. A field that stored
// an instant would be answering a different question from the one a person filling in "due for
// review" is asking, and two clients in two zones would disagree about which day it is.
func (d CustomFieldDefinition) validDate(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if _, err := time.Parse(time.DateOnly, text); err != nil {
		return nil, d.valueError("fields.value_not_a_date", nil)
	}
	return text, nil
}

func (d CustomFieldDefinition) validSelect(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if !slices.Contains(d.Options, text) {
		// The value is echoed back. It is one a client chose from a list this server published, so
		// it carries nothing the server did not already say - and a client that sent a stale option
		// has to see which one.
		return nil, d.valueError("fields.value_not_an_option", map[string]string{"value": text})
	}
	return text, nil
}

func (d CustomFieldDefinition) validMultiSelect(value any) (any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if len(raw) == 0 {
		return nil, d.emptyOrCleared()
	}
	if len(raw) > MaxCustomFieldSelections {
		return nil, d.valueError("fields.value_too_many",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldSelections)})
	}

	chosen := make([]any, 0, len(raw))
	seen := make([]string, 0, len(raw))
	for _, element := range raw {
		text, ok := element.(string)
		if !ok {
			return nil, d.typeMismatch(element)
		}
		if !slices.Contains(d.Options, text) {
			return nil, d.valueError("fields.value_not_an_option", map[string]string{"value": text})
		}
		if slices.Contains(seen, text) {
			// The same choice twice is one choice. Dropped rather than refused: a client that sent
			// it meant the set it describes, and the set is what is stored.
			continue
		}
		seen = append(seen, text)
		chosen = append(chosen, text)
	}
	return chosen, nil
}

func (d CustomFieldDefinition) validBool(value any) (any, error) {
	flag, ok := value.(bool)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	return flag, nil
}

// validUser checks the shape and nothing else. Whether the account exists and belongs to this
// workspace is a question about a second person that only the application layer can ask, and it
// asks it - the same question an assignment asks (items.account_without_access).
func (d CustomFieldDefinition) validUser(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if _, err := shared.ParseID(text); err != nil {
		return nil, d.valueError("fields.value_not_an_account", nil)
	}
	return text, nil
}

// validURL refuses anything that is not an absolute http or https address.
//
// The scheme list is closed on purpose: `javascript:` and `data:` are the two a client would
// eventually render as a link, and a field that stored them would be a stored cross-site script
// waiting for the first frontend that trusts its own data (T-11's reasoning, one field further in).
func (d CustomFieldDefinition) validURL(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, d.emptyOrCleared()
	}
	if utf8.RuneCountInString(text) > MaxCustomFieldURLLength {
		return nil, d.valueError("fields.value_too_long",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldURLLength)})
	}

	parsed, err := url.Parse(text)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, d.valueError("fields.value_not_a_url", nil)
	}
	return text, nil
}

// emptyOrCleared is the one answer for a value that says nothing: clear the key instead. Sending
// "" and sending null would otherwise be two ways to mean the same thing, only one of which a
// required field refuses.
func (d CustomFieldDefinition) emptyOrCleared() error {
	return d.valueError("fields.value_empty", nil)
}

// typeMismatch names what the field wanted. Never what arrived: the value is user content, and
// rule 10 keeps it out of a message the trail may end up carrying.
func (d CustomFieldDefinition) typeMismatch(any) error {
	return d.valueError("fields.value_type_mismatch", map[string]string{"kind": string(d.Kind)})
}

// valueError points at the value rather than at the whole request, and names the key so that a
// client writing several fields knows which one it got wrong.
func (d CustomFieldDefinition) valueError(code string, params map[string]string) error {
	if params == nil {
		params = map[string]string{}
	}
	params["key"] = d.Key

	return shared.ErrValidation.WithDetail(code).WithParams(params).
		WithFields(shared.FieldError{Path: "/value", Code: code, Params: params})
}
