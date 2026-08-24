// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	fieldID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	fieldTenant = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	fieldColl   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f3")
	fieldNow    = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
)

func definition(t *testing.T, kind work.CustomFieldKind, options ...string) work.CustomFieldDefinition {
	t.Helper()

	built, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, CollectionID: fieldColl,
		Key: "priority", Kind: kind, Options: options,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow,
	})
	if err != nil {
		t.Fatalf("building a %s definition: %v", kind, err)
	}
	return built
}

func TestADefinitionIsBuiltFromItsKeyKindAndScope(t *testing.T) {
	built := definition(t, work.CustomFieldSelect, " high ", "low")

	if built.Key != "priority" || built.Kind != work.CustomFieldSelect {
		t.Fatalf("the definition is %+v", built)
	}
	// The options are trimmed on the way in, so that "high" and "high " are one option rather than
	// two a person cannot tell apart.
	if len(built.Options) != 2 || built.Options[0] != "high" {
		t.Errorf("the options are %v", built.Options)
	}
	if built.Version != 1 || built.CreatedAt != fieldNow || built.IsTenantWide() {
		t.Errorf("the definition is %+v", built)
	}
}

func TestAKeyIsAnIdentifierAndNothingElse(t *testing.T) {
	cases := []struct {
		name, key, detail string
	}{
		{"empty", "  ", "fields.key_required"},
		{"leading digit", "1priority", "fields.key_malformed"},
		{"upper case", "Priority", "fields.key_malformed"},
		{"a space inside", "my priority", "fields.key_malformed"},
		{"a dot, which a filter path already uses", "custom.priority", "fields.key_malformed"},
		{"too long", "a123456789012345678901234567890123456789012345678901", "fields.key_too_long"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
				ID: fieldID, TenantID: fieldTenant, Key: c.key, Kind: work.CustomFieldText,
				AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow,
			})
			if detail := shared.AsError(err).DetailCode; detail != c.detail {
				t.Fatalf("detail %q, want %s", detail, c.detail)
			}
		})
	}
}

func TestOnlyAChoiceKindCarriesOptions(t *testing.T) {
	// A SELECT with none offers nothing to pick from.
	_, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, Key: "priority", Kind: work.CustomFieldSelect,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow,
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.options_required" {
		t.Errorf("a SELECT without options answered %q", detail)
	}

	// Options on a BOOL are a client that misunderstood the field, and storing them would make the
	// misunderstanding survive.
	_, err = work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, Key: "done_twice", Kind: work.CustomFieldBool,
		Options: []string{"yes"}, AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow,
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.options_not_applicable" {
		t.Errorf("a BOOL with options answered %q", detail)
	}
}

func TestTwoOptionsThatReadTheSameAreRefused(t *testing.T) {
	_, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, Key: "priority", Kind: work.CustomFieldSelect,
		Options: []string{"high", " high"}, AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow,
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.option_duplicated" {
		t.Fatalf("detail %q, want fields.option_duplicated", detail)
	}
}

func TestADefinitionNoTypeCarriesIsRefused(t *testing.T) {
	_, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, Key: "priority", Kind: work.CustomFieldText,
		AppliesTo: nil, Now: fieldNow,
	})
	if detail := shared.AsError(err).DetailCode; detail != "fields.applies_to_required" {
		t.Fatalf("detail %q, want fields.applies_to_required", detail)
	}
}

// The table the whole feature rests on: what each kind takes, and what it refuses.
func TestEachKindAcceptsItsOwnShapeAndNothingElse(t *testing.T) {
	cases := []struct {
		name    string
		kind    work.CustomFieldKind
		options []string
		value   any
		want    any
		detail  string
	}{
		{name: "text", kind: work.CustomFieldText, value: "  urgent  ", want: "urgent"},
		{
			name: "text refuses a number", kind: work.CustomFieldText, value: float64(3),
			detail: "fields.value_type_mismatch",
		},
		{name: "number", kind: work.CustomFieldNumber, value: float64(3.5), want: float64(3.5)},
		{
			// A client that sent "3" meant a string, and guessing turns a typo into stored data.
			name: "number refuses a string", kind: work.CustomFieldNumber, value: "3",
			detail: "fields.value_type_mismatch",
		},
		{name: "date", kind: work.CustomFieldDate, value: "2026-08-24", want: "2026-08-24"},
		{
			name: "date refuses an instant", kind: work.CustomFieldDate,
			value: "2026-08-24T09:00:00Z", detail: "fields.value_not_a_date",
		},
		{
			name: "select", kind: work.CustomFieldSelect, options: []string{"high", "low"},
			value: "high", want: "high",
		},
		{
			name: "select refuses what it does not offer", kind: work.CustomFieldSelect,
			options: []string{"high"}, value: "medium", detail: "fields.value_not_an_option",
		},
		{
			name: "multi select", kind: work.CustomFieldMultiSelect, options: []string{"a", "b"},
			value: []any{"a", "b"}, want: []any{"a", "b"},
		},
		{
			name: "multi select folds a repeat", kind: work.CustomFieldMultiSelect,
			options: []string{"a"}, value: []any{"a", "a"}, want: []any{"a"},
		},
		{
			name: "multi select refuses a bare string", kind: work.CustomFieldMultiSelect,
			options: []string{"a"}, value: "a", detail: "fields.value_type_mismatch",
		},
		{name: "bool", kind: work.CustomFieldBool, value: true, want: true},
		{
			name: "bool refuses a string", kind: work.CustomFieldBool, value: "true",
			detail: "fields.value_type_mismatch",
		},
		{
			name: "user", kind: work.CustomFieldUser,
			value: fieldTenant.String(), want: fieldTenant.String(),
		},
		{
			name: "user refuses what is not an identifier", kind: work.CustomFieldUser,
			value: "anna", detail: "fields.value_not_an_account",
		},
		{
			name: "url", kind: work.CustomFieldURL,
			value: "https://hubtask.eu/x", want: "https://hubtask.eu/x",
		},
		{
			// The scheme list is closed: a stored `javascript:` is a cross-site script waiting for
			// the first frontend that trusts its own data.
			name: "url refuses a script", kind: work.CustomFieldURL,
			value: "javascript:alert(1)", detail: "fields.value_not_a_url",
		},
		{
			name: "url refuses a bare host", kind: work.CustomFieldURL, value: "hubtask.eu",
			detail: "fields.value_not_a_url",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			field := definition(t, c.kind, c.options...)

			got, err := field.ValidateValue(c.value)
			if c.detail != "" {
				if detail := shared.AsError(err).DetailCode; detail != c.detail {
					t.Fatalf("detail %q, want %s", detail, c.detail)
				}
				// Every refusal names the key, so a client writing several fields knows which one.
				if params := shared.AsError(err).Params; params["key"] != "priority" {
					t.Errorf("the refusal names %v", params)
				}
				return
			}
			if err != nil {
				t.Fatalf("the value was refused: %v", err)
			}
			if !sameValue(got, c.want) {
				t.Errorf("the stored value is %#v, want %#v", got, c.want)
			}
		})
	}
}

// Clearing is null and nothing else. An empty string is refused rather than treated as a clear, so
// that a required field cannot be satisfied by sending "".
func TestClearingIsNullAndAnEmptyStringIsNeither(t *testing.T) {
	field := definition(t, work.CustomFieldText)

	got, err := field.ValidateValue(nil)
	if err != nil || got != nil {
		t.Fatalf("clearing answered %#v (%v)", got, err)
	}

	if _, err := field.ValidateValue("   "); shared.AsError(err).DetailCode != "fields.value_empty" {
		t.Errorf("an empty text answered %v", err)
	}
}

func TestARequiredFieldRefusesToBeCleared(t *testing.T) {
	field := definition(t, work.CustomFieldText)
	field.IsRequired = true

	if _, err := field.ValidateValue(nil); shared.AsError(err).DetailCode != "fields.value_required" {
		t.Fatalf("clearing a required field answered %v", err)
	}
}

func TestAnUpdateReportsWhatMovedAndNothingElse(t *testing.T) {
	field := definition(t, work.CustomFieldSelect, "high", "low")
	options := []string{"high", "low", "medium"}
	required := true

	updated, changes, err := field.Updated(work.CustomFieldAttributes{
		Options: &options, IsRequired: &required,
	}, fieldNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("the changes are %+v", changes)
	}
	if len(updated.Options) != 3 || !updated.IsRequired {
		t.Errorf("the definition is %+v", updated)
	}
	if updated.UpdatedAt != fieldNow.Add(time.Hour) {
		t.Errorf("the stamp is %v", updated.UpdatedAt)
	}

	// The same values again move nothing, so no version is spent and nothing is announced.
	_, none, err := updated.Updated(work.CustomFieldAttributes{
		Options: &options, IsRequired: &required,
	}, fieldNow.Add(2*time.Hour))
	if err != nil || len(none) != 0 {
		t.Errorf("a repeat reported %+v (%v)", none, err)
	}
}

// Narrowing the options does not rewrite what the entries already hold - that would be the
// unbounded write the soft delete exists to avoid - but the next write of the field is refused
// unless it picks from the new list.
func TestNarrowingTheOptionsBindsTheNextWriteOnly(t *testing.T) {
	field := definition(t, work.CustomFieldSelect, "high", "low")
	narrowed := []string{"high"}

	updated, _, err := field.Updated(work.CustomFieldAttributes{Options: &narrowed}, fieldNow)
	if err != nil {
		t.Fatalf("narrowing failed: %v", err)
	}
	if _, err := updated.ValidateValue("low"); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a value that is no longer offered was accepted: %v", err)
	}
}

func TestADeletedDefinitionIsOutOfUseAndTheDeletionIsIdempotent(t *testing.T) {
	field := definition(t, work.CustomFieldText)

	deleted, changes, err := field.Deleted(fieldNow)
	if err != nil || len(changes) != 1 {
		t.Fatalf("the deletion answered %+v (%v)", changes, err)
	}
	if !deleted.IsDeleted() {
		t.Fatal("the definition is not marked")
	}

	_, none, err := deleted.Deleted(fieldNow.Add(time.Hour))
	if err != nil || len(none) != 0 {
		t.Errorf("a second deletion reported %+v (%v)", none, err)
	}
	// And it cannot be edited afterwards: a conflict rather than a validation failure, because
	// nothing about the request is wrong - the definition is.
	required := true
	if _, _, err := deleted.Updated(
		work.CustomFieldAttributes{IsRequired: &required}, fieldNow,
	); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("editing a deleted definition answered %v", err)
	}
}

func TestAKindNobodyDefinedIsRefusedByName(t *testing.T) {
	if _, err := work.ParseCustomFieldKind("COLOUR"); shared.AsError(err).DetailCode != "fields.kind_unknown" {
		t.Fatalf("an unknown kind answered %v", err)
	}
	for _, kind := range work.CustomFieldKinds() {
		if _, err := work.ParseCustomFieldKind(string(kind)); err != nil {
			t.Errorf("%s is in the list and is refused: %v", kind, err)
		}
	}
}

// sameValue compares two stored values, arrays included.
func sameValue(got, want any) bool {
	gotList, isList := got.([]any)
	wantList, wantIsList := want.([]any)
	if isList != wantIsList {
		return false
	}
	if !isList {
		return got == want
	}
	if len(gotList) != len(wantList) {
		return false
	}
	for i := range gotList {
		if gotList[i] != wantList[i] {
			return false
		}
	}
	return true
}
