// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"math"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// CustomFieldKind is what a value for a field looks like: the eight the schema has carried since
// `0001_init` and those the suite's field registry adds (domain-model.md §3.5, F-2).
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
	// CustomFieldMoney is an amount in the definition's one currency. The currency is the
	// definition's and never the value's: amounts in one field that could each name their own
	// currency could not be summed, compared or sorted.
	CustomFieldMoney CustomFieldKind = "MONEY"
	// CustomFieldDuration is a span of time in whole minutes - an estimate, a time box.
	CustomFieldDuration CustomFieldKind = "DURATION"
	// CustomFieldReference points at another entry of the workspace (P-19).
	CustomFieldReference CustomFieldKind = "REFERENCE"
	// CustomFieldUserSet is several people - reviewers, approvers.
	CustomFieldUserSet CustomFieldKind = "USER_SET"
	// CustomFieldRichText is formatted text in the notes' Markdown. Never compared, sorted or
	// grouped: what a person formats is prose, not a value.
	CustomFieldRichText CustomFieldKind = "RICH_TEXT"
	// CustomFieldCalculated is a number the server computes with the definition's formula from the
	// entry's own fields, whenever they are written. Nobody writes one.
	CustomFieldCalculated CustomFieldKind = "CALCULATED"
	// CustomFieldDateTime is a moment, stored in UTC to the second. DATE stays the calendar day.
	CustomFieldDateTime CustomFieldKind = "DATE_TIME"
	// CustomFieldGroup is one of the workspace's groups - the team that answers, the owners.
	CustomFieldGroup CustomFieldKind = "GROUP"
	// CustomFieldCascadingSelect is a choice of one or two levels: its options are paths written
	// `Parent` and `Parent/Child`, and a value is one of them.
	CustomFieldCascadingSelect CustomFieldKind = "CASCADING_SELECT"
	// CustomFieldIPAddress is one IPv4 or IPv6 address, in its canonical spelling.
	CustomFieldIPAddress CustomFieldKind = "IP_ADDRESS"
	// CustomFieldFile is one of the entry's own attachments, by its media identifier.
	CustomFieldFile CustomFieldKind = "FILE"
	// CustomFieldLabel is a set of free words nobody defines first, as a page's labels are.
	CustomFieldLabel CustomFieldKind = "LABEL"
	// CustomFieldStatus is one state of a record from the options, which are listed in the order a
	// record passes through them. It drives nothing: a status that moves work is a workflow's.
	CustomFieldStatus CustomFieldKind = "STATUS"
)

var customFieldKinds = [...]CustomFieldKind{
	CustomFieldText, CustomFieldNumber, CustomFieldDate, CustomFieldSelect,
	CustomFieldMultiSelect, CustomFieldBool, CustomFieldUser, CustomFieldURL,
	CustomFieldMoney, CustomFieldDuration, CustomFieldReference, CustomFieldUserSet,
	CustomFieldRichText, CustomFieldCalculated, CustomFieldDateTime, CustomFieldGroup,
	CustomFieldCascadingSelect, CustomFieldIPAddress, CustomFieldFile, CustomFieldLabel,
	CustomFieldStatus,
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
	return k == CustomFieldSelect || k == CustomFieldMultiSelect || k == CustomFieldCascadingSelect ||
		k == CustomFieldStatus
}

// TakesFormula reports whether the kind's definition carries a formula.
func (k CustomFieldKind) TakesFormula() bool { return k == CustomFieldCalculated }

// IsFormulaInput reports whether a formula may read a field of the kind: the numbers, and a
// boolean as one. Never another CALCULATED field, so no formula waits on another and none can
// wait on itself.
func (k CustomFieldKind) IsFormulaInput() bool {
	return k == CustomFieldNumber || k == CustomFieldMoney || k == CustomFieldDuration || k == CustomFieldBool
}

// TakesCurrency reports whether the kind's definition names a currency.
func (k CustomFieldKind) TakesCurrency() bool { return k == CustomFieldMoney }

// IsSet reports whether one value of the kind holds several elements.
func (k CustomFieldKind) IsSet() bool {
	return k == CustomFieldMultiSelect || k == CustomFieldUserSet || k == CustomFieldLabel
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
	// MaxCustomFieldRichTextLength bounds a RICH_TEXT value: a few pages, the size of a long note
	// rather than of a document - a document is a page (M3), not a field.
	MaxCustomFieldRichTextLength = 20000
	// MaxCustomFieldMoneyDecimals is the finest fraction an amount may carry. Four places hold
	// every currency's minor unit (ISO 4217 has none finer) and a unit price beside it.
	MaxCustomFieldMoneyDecimals = 4
	// MaxCustomFieldMoneyMagnitude bounds an amount's size: inside it float64 still holds four
	// decimal places exactly, so what is stored is what was sent.
	MaxCustomFieldMoneyMagnitude = 1e11
	// MaxCustomFieldDurationMinutes bounds a DURATION: ten years of minutes.
	MaxCustomFieldDurationMinutes = 10 * 366 * 24 * 60
	// MaxCustomFieldPeople bounds one USER_SET value, as MaxCustomFieldSelections bounds a
	// MULTI_SELECT.
	MaxCustomFieldPeople = 50
	// MaxCustomFieldFormulaLength bounds a CALCULATED field's formula, as the column's CHECK does.
	MaxCustomFieldFormulaLength = 2000
	// MaxCustomFieldsPerItem bounds how many keys one entry may carry. The column is one jsonb
	// document that every read of the entry carries, so its size is the entry's size.
	MaxCustomFieldsPerItem = 100
	// MaxCustomFieldLabelLength and MaxCustomFieldLabels bound a LABEL value as a page's labels
	// are bounded, so a label is one token in a filter and a facet.
	MaxCustomFieldLabelLength = 40
	MaxCustomFieldLabels      = 20
)

// CascadingSeparator joins a CASCADING_SELECT option's parent and child.
const CascadingSeparator = "/"

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
	// Currency is the ISO 4217 code of a MONEY field's amounts, empty for every other kind. Fixed
	// once defined, like the kind: a changed currency would reinterpret every stored amount.
	Currency string
	// Formula is a CALCULATED field's expression over `entry.custom_fields`, empty for every other
	// kind. Fixed once defined, like the currency: a changed formula would rewrite every entry's
	// value at once. Whether it compiles is the use case's question, which holds the compiler.
	Formula string
	// IsRequired is enforced when the field is written and never retroactively: making a field
	// required does not make the entries that predate it invalid, and a rule that did would make
	// every one of them unsaveable at once.
	IsRequired bool
	// Restricted says only holders of the field's permission (FIELD_WRITE:<key>) write it
	// (UC-ID-27); the shipped owner and administrators hold every one. A calculated field is
	// written by the server and is never restricted.
	Restricted bool
	// ReadRestricted narrows the reading too: only holders of FIELD_READ:<key> or FIELD_WRITE:<key>
	// see the value, in every answer (UC-ID-27 check 3). It implies Restricted - a field hidden from
	// a reader that the same reader could still write would be overwritten blind.
	ReadRestricted bool
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
	Currency     string
	Formula      string
	IsRequired   bool
	Restricted   bool
	// ReadRestricted implies Restricted.
	ReadRestricted bool
	AppliesTo      []ItemType
	Now            time.Time

	// Text brings the options to normal form C before they are bounded and stored (i18n-l10n.md
	// §5): an option is a value a person typed once and every entry then picks from, so a choice
	// has to meet it in one form. NewWorkItemInput says why it is handed in.
	Text text.Normalizer
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
	options, err := customFieldOptions(in.Kind, in.Options, in.Text)
	if err != nil {
		return CustomFieldDefinition{}, err
	}
	currency, err := customFieldCurrency(in.Kind, in.Currency)
	if err != nil {
		return CustomFieldDefinition{}, err
	}
	formula, err := customFieldFormula(in.Kind, in.Formula, in.IsRequired)
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
	restricted := in.Restricted || in.ReadRestricted
	if restricted && in.Kind == CustomFieldCalculated {
		return CustomFieldDefinition{}, customFieldError("/restricted", "fields.calculated", nil)
	}

	return CustomFieldDefinition{
		ID: in.ID, TenantID: in.TenantID, CollectionID: in.CollectionID,
		Key: key, Kind: in.Kind, Options: options, Currency: currency, Formula: formula,
		IsRequired: in.IsRequired, Restricted: restricted, ReadRestricted: in.ReadRestricted,
		AppliesTo: appliesTo, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
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
	Restricted *bool
	// ReadRestricted true restricts the writing as well; Restricted false lifts both.
	ReadRestricted *bool
	AppliesTo      *[]ItemType
}

// IsEmpty reports whether the update asks for nothing at all.
func (a CustomFieldAttributes) IsEmpty() bool {
	return a.Options == nil && a.IsRequired == nil && a.Restricted == nil && a.ReadRestricted == nil && a.AppliesTo == nil
}

// The field names an update reports its changes under.
const (
	FieldOptions    = "options"
	FieldIsRequired = "is_required"
	FieldRestricted = "restricted"
	// FieldReadRestricted is the change of whether the field is hidden from readers.
	FieldReadRestricted = "read_restricted"
	FieldAppliesTo      = "applies_to"
)

// Updated applies a change and reports which fields moved. Nothing that did not move is in the
// result, and a request that changes nothing returns the definition untouched.
//
// Narrowing the options does not rewrite the entries holding a value that is no longer offered.
// That would be the unbounded write the soft delete exists to avoid, and the value is not wrong -
// it was permitted when it was written. It stops being offered, and the next write of that field
// is refused unless it picks from the new list.
//
// The normaliser is handed in for the reason NewCustomFieldDefinition takes one (i18n-l10n.md §5).
func (d CustomFieldDefinition) Updated(
	attributes CustomFieldAttributes, form text.Normalizer, at time.Time,
) (CustomFieldDefinition, []FieldChange, error) {
	if err := d.EnsureEditable(); err != nil {
		return CustomFieldDefinition{}, nil, err
	}

	var changes []FieldChange

	if attributes.Options != nil {
		options, err := customFieldOptions(d.Kind, *attributes.Options, form)
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

	if attributes.IsRequired != nil && *attributes.IsRequired && d.Kind == CustomFieldCalculated {
		return CustomFieldDefinition{}, nil, customFieldError("/is_required", "fields.calculated", nil)
	}
	if attributes.IsRequired != nil && *attributes.IsRequired != d.IsRequired {
		changes = append(changes, FieldChange{
			Field: FieldIsRequired,
			From:  strconv.FormatBool(d.IsRequired), To: strconv.FormatBool(*attributes.IsRequired),
		})
		d.IsRequired = *attributes.IsRequired
	}

	restricted, readRestricted := d.Restricted, d.ReadRestricted
	if attributes.Restricted != nil {
		restricted = *attributes.Restricted
		readRestricted = readRestricted && restricted
	}
	if attributes.ReadRestricted != nil {
		readRestricted = *attributes.ReadRestricted
		restricted = restricted || readRestricted
	}
	if restricted && d.Kind == CustomFieldCalculated {
		return CustomFieldDefinition{}, nil, customFieldError("/restricted", "fields.calculated", nil)
	}
	if restricted != d.Restricted {
		changes = append(changes, FieldChange{
			Field: FieldRestricted, From: strconv.FormatBool(d.Restricted), To: strconv.FormatBool(restricted),
		})
		d.Restricted = restricted
	}
	if readRestricted != d.ReadRestricted {
		changes = append(changes, FieldChange{
			Field: FieldReadRestricted,
			From:  strconv.FormatBool(d.ReadRestricted), To: strconv.FormatBool(readRestricted),
		})
		d.ReadRestricted = readRestricted
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
func customFieldOptions(kind CustomFieldKind, raw []string, form text.Normalizer) ([]string, error) {
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
		value, err := shared.NFC(strings.TrimSpace(option), form)
		if err != nil {
			return nil, err
		}
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
	if kind == CustomFieldCascadingSelect {
		if err := cascadingOptions(options); err != nil {
			return nil, err
		}
	}
	return options, nil
}

// cascadingOptions holds a two-level choice's paths to one or two non-empty parts, each child under
// a parent the list itself offers - a child whose parent is not offered could be picked only by
// typing it.
func cascadingOptions(options []string) error {
	for _, option := range options {
		parts := strings.Split(option, CascadingSeparator)
		if len(parts) > 2 || slices.ContainsFunc(parts, func(part string) bool { return strings.TrimSpace(part) == "" }) {
			return customFieldError("/options", "fields.option_path_malformed", nil)
		}
		if len(parts) == 2 && !slices.Contains(options, parts[0]) {
			return customFieldError("/options", "fields.option_without_parent",
				map[string]string{"option": option})
		}
	}
	return nil
}

// customFieldCurrency insists a MONEY field names its currency and every other kind names none.
// The shape is checked, not the list: ISO 4217 changes, and a currency the server's list did not
// know yet would be one a workspace cannot record.
func customFieldCurrency(kind CustomFieldKind, raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if !kind.TakesCurrency() {
		if code != "" {
			return "", customFieldError("/currency", "fields.currency_not_applicable",
				map[string]string{"kind": string(kind)})
		}
		return "", nil
	}
	if code == "" {
		return "", customFieldError("/currency", "fields.currency_required", nil)
	}
	if len(code) != 3 || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return "", customFieldError("/currency", "fields.currency_malformed", nil)
	}
	return code, nil
}

// customFieldFormula holds a formula to its kind and its length. A CALCULATED field is never
// required: a person cannot fill it in, so a required one would refuse every write.
func customFieldFormula(kind CustomFieldKind, raw string, required bool) (string, error) {
	formula := strings.TrimSpace(raw)
	if !kind.TakesFormula() {
		if formula != "" {
			return "", customFieldError("/formula", "fields.formula_not_applicable", map[string]string{"kind": string(kind)})
		}
		return "", nil
	}
	if formula == "" {
		return "", customFieldError("/formula", "fields.formula_required", nil)
	}
	if utf8.RuneCountInString(formula) > MaxCustomFieldFormulaLength {
		return "", customFieldError("/formula", "fields.formula_invalid", nil)
	}
	if required {
		return "", customFieldError("/is_required", "fields.calculated", nil)
	}
	return formula, nil
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
//
// The normaliser is for the kinds that hold text a person typed (i18n-l10n.md §5): a TEXT value is
// stored in normal form C like a note, and a choice is brought to the form the options were stored
// in before it is looked for among them.
func (d CustomFieldDefinition) ValidateValue(value any, form text.Normalizer) (any, error) {
	if d.Kind == CustomFieldCalculated {
		// The server's value alone: a person or a client neither sets nor clears it.
		return nil, d.valueError("fields.calculated", nil)
	}
	if value == nil {
		if d.IsRequired {
			return nil, d.valueError("fields.value_required", nil)
		}
		return nil, nil
	}

	switch d.Kind {
	case CustomFieldText:
		return d.validText(value, form)
	case CustomFieldNumber:
		return d.validNumber(value)
	case CustomFieldDate:
		return d.validDate(value)
	case CustomFieldSelect, CustomFieldCascadingSelect, CustomFieldStatus:
		return d.validSelect(value, form)
	case CustomFieldMultiSelect:
		return d.validMultiSelect(value, form)
	case CustomFieldBool:
		return d.validBool(value)
	case CustomFieldUser:
		return d.validUser(value)
	case CustomFieldURL:
		return d.validURL(value)
	case CustomFieldMoney:
		return d.validMoney(value)
	case CustomFieldDuration:
		return d.validDuration(value)
	case CustomFieldReference:
		return d.validReference(value)
	case CustomFieldUserSet:
		return d.validUserSet(value)
	case CustomFieldRichText:
		return d.validRichText(value, form)
	case CustomFieldDateTime:
		return d.validMoment(value)
	case CustomFieldGroup:
		return d.validIdentifier(value, "fields.value_not_a_group")
	case CustomFieldIPAddress:
		return d.validAddress(value)
	case CustomFieldFile:
		return d.validIdentifier(value, "fields.value_not_an_attachment")
	case CustomFieldLabel:
		return d.validLabels(value)
	default:
		// A kind no constructor produces. A defect rather than input (security.md §9).
		return nil, shared.ErrInternal.WithDetail("fields.kind_unhandled")
	}
}

func (d CustomFieldDefinition) validText(value any, form text.Normalizer) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	text, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return nil, err
	}
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

func (d CustomFieldDefinition) validSelect(value any, form text.Normalizer) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	text, err := shared.NFC(raw, form)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(d.Options, text) {
		// The value is echoed back. It is one a client chose from a list this server published, so
		// it carries nothing the server did not already say - and a client that sent a stale option
		// has to see which one.
		return nil, d.valueError("fields.value_not_an_option", map[string]string{"value": text})
	}
	return text, nil
}

func (d CustomFieldDefinition) validMultiSelect(value any, form text.Normalizer) (any, error) {
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
		typed, ok := element.(string)
		if !ok {
			return nil, d.typeMismatch(element)
		}
		text, err := shared.NFC(typed, form)
		if err != nil {
			return nil, err
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

// validMoney keeps an amount exact to MaxCustomFieldMoneyDecimals places. Finer is refused rather
// than rounded: a rounding the server chose is a sum nobody can reconcile.
func (d CustomFieldDefinition) validMoney(value any) (any, error) {
	amount, ok := value.(float64)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, d.valueError("fields.value_not_finite", nil)
	}
	if math.Abs(amount) >= MaxCustomFieldMoneyMagnitude {
		return nil, d.valueError("fields.value_too_large", nil)
	}
	scaled := amount * math.Pow10(MaxCustomFieldMoneyDecimals)
	if math.Abs(scaled-math.Round(scaled)) > 1e-6 {
		return nil, d.valueError("fields.value_too_precise",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldMoneyDecimals)})
	}
	return math.Round(scaled) / math.Pow10(MaxCustomFieldMoneyDecimals), nil
}

// validDuration takes whole minutes, never negative. Minutes rather than seconds because every
// estimate and time box a person writes is in them, and an integer because a fraction of a minute
// is a precision no plan has.
func (d CustomFieldDefinition) validDuration(value any) (any, error) {
	minutes, ok := value.(float64)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if math.IsNaN(minutes) || math.IsInf(minutes, 0) || minutes != math.Trunc(minutes) {
		return nil, d.valueError("fields.value_not_whole_minutes", nil)
	}
	if minutes < 0 || minutes > MaxCustomFieldDurationMinutes {
		return nil, d.valueError("fields.value_out_of_range",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldDurationMinutes)})
	}
	return minutes, nil
}

// validReference checks the shape. Whether the entry exists and the writer may see it is the
// application's question, asked the way USER's is.
func (d CustomFieldDefinition) validReference(value any) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if _, err := shared.ParseID(text); err != nil {
		return nil, d.valueError("fields.value_not_an_entry", nil)
	}
	return text, nil
}

// validUserSet is USER's shape check over a list, with MULTI_SELECT's bounds and its rule that the
// same element twice is one element.
func (d CustomFieldDefinition) validUserSet(value any) (any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if len(raw) == 0 {
		return nil, d.emptyOrCleared()
	}
	if len(raw) > MaxCustomFieldPeople {
		return nil, d.valueError("fields.value_too_many",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldPeople)})
	}
	chosen := make([]any, 0, len(raw))
	seen := make([]string, 0, len(raw))
	for _, element := range raw {
		text, ok := element.(string)
		if !ok {
			return nil, d.typeMismatch(element)
		}
		id, err := shared.ParseID(text)
		if err != nil {
			return nil, d.valueError("fields.value_not_an_account", nil)
		}
		canonical := id.String()
		if slices.Contains(seen, canonical) {
			continue
		}
		seen = append(seen, canonical)
		chosen = append(chosen, canonical)
	}
	return chosen, nil
}

// validRichText is TEXT's rule with the larger bound: Markdown in normal form C.
func (d CustomFieldDefinition) validRichText(value any, form text.Normalizer) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	body, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return nil, err
	}
	if body == "" {
		return nil, d.emptyOrCleared()
	}
	if utf8.RuneCountInString(body) > MaxCustomFieldRichTextLength {
		return nil, d.valueError("fields.value_too_long",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldRichTextLength)})
	}
	return body, nil
}

// validMoment takes an RFC 3339 moment and stores it in UTC to the second, so that one moment has
// one spelling in the document an equality filter compares.
func (d CustomFieldDefinition) validMoment(value any) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	moment, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return nil, d.valueError("fields.value_not_a_moment", nil)
	}
	return moment.UTC().Truncate(time.Second).Format(time.RFC3339), nil
}

// validIdentifier checks the shape of a GROUP's or a FILE's identifier. Whether the group is the
// workspace's or the attachment the entry's is the application's question, asked as USER's is.
func (d CustomFieldDefinition) validIdentifier(value any, code string) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	id, err := shared.ParseID(raw)
	if err != nil {
		return nil, d.valueError(code, nil)
	}
	return id.String(), nil
}

// validAddress takes one address, IPv4 or IPv6, with no prefix and no zone: a network is a range
// of addresses and a zone names an interface of one machine, neither of which a register's row
// for a device holds.
func (d CustomFieldDefinition) validAddress(value any) (any, error) {
	raw, ok := value.(string)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || address.Zone() != "" {
		return nil, d.valueError("fields.value_not_an_address", nil)
	}
	return address.String(), nil
}

// validLabels takes a set of words in lower case: letters, digits, `-`, `_` and `.`, the rule a
// page's labels follow, so that a label is one token in a filter.
func (d CustomFieldDefinition) validLabels(value any) (any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, d.typeMismatch(value)
	}
	if len(raw) == 0 {
		return nil, d.emptyOrCleared()
	}
	chosen := make([]any, 0, len(raw))
	seen := make([]string, 0, len(raw))
	for _, element := range raw {
		typed, ok := element.(string)
		if !ok {
			return nil, d.typeMismatch(element)
		}
		label := strings.ToLower(strings.TrimSpace(typed))
		if !validFieldLabel(label) {
			return nil, d.valueError("fields.value_not_a_label",
				map[string]string{"maximum": strconv.Itoa(MaxCustomFieldLabelLength)})
		}
		if slices.Contains(seen, label) {
			continue
		}
		seen = append(seen, label)
		chosen = append(chosen, label)
	}
	if len(chosen) > MaxCustomFieldLabels {
		return nil, d.valueError("fields.value_too_many",
			map[string]string{"maximum": strconv.Itoa(MaxCustomFieldLabels)})
	}
	return chosen, nil
}

func validFieldLabel(label string) bool {
	if label == "" || utf8.RuneCountInString(label) > MaxCustomFieldLabelLength {
		return false
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// GroupOf returns the group a GROUP value names, for the application's check that it is the
// workspace's. False for every other kind and for a cleared value.
func (d CustomFieldDefinition) GroupOf(value any) (shared.ID, bool) {
	return d.identifierOf(CustomFieldGroup, value)
}

// AttachmentOf returns the media a FILE value names, for the application's check that the entry
// carries it.
func (d CustomFieldDefinition) AttachmentOf(value any) (shared.ID, bool) {
	return d.identifierOf(CustomFieldFile, value)
}

func (d CustomFieldDefinition) identifierOf(kind CustomFieldKind, value any) (shared.ID, bool) {
	text, ok := value.(string)
	if d.Kind != kind || !ok {
		return "", false
	}
	id, err := shared.ParseID(text)
	return id, err == nil
}

// Accounts returns the accounts a USER or USER_SET value names, for the application's check that
// each of them can see the entry. Empty for every other kind and for a cleared value.
func (d CustomFieldDefinition) Accounts(value any) []shared.ID {
	var texts []string
	switch typed := value.(type) {
	case string:
		if d.Kind == CustomFieldUser {
			texts = []string{typed}
		}
	case []any:
		if d.Kind == CustomFieldUserSet {
			for _, element := range typed {
				if text, ok := element.(string); ok {
					texts = append(texts, text)
				}
			}
		}
	}
	accounts := make([]shared.ID, 0, len(texts))
	for _, text := range texts {
		if id, err := shared.ParseID(text); err == nil {
			accounts = append(accounts, id)
		}
	}
	return accounts
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

// CalculatedValue is what a CALCULATED field holds for a formula's result: a finite number rounded
// to four decimal places, as a MONEY amount is, or nothing where the formula could not answer.
func CalculatedValue(number float64, answered bool) any {
	if !answered || math.IsNaN(number) || math.IsInf(number, 0) || math.Abs(number) >= MaxCustomFieldMoneyMagnitude {
		return nil
	}
	return math.Round(number*1e4) / 1e4
}
