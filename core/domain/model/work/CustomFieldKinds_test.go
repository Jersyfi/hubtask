// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"math"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func typedDefinition(
	t *testing.T, kind work.CustomFieldKind, currency string,
) (work.CustomFieldDefinition, error) {
	t.Helper()
	return work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: fieldID, TenantID: fieldTenant, CollectionID: fieldColl,
		Key: "priority", Kind: kind, Currency: currency,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow, Text: text.Composing{},
	})
}

// UC-WRK-12 check 1: a MONEY field names its currency, and no other kind names one.
func TestOnlyAMoneyFieldNamesACurrency(t *testing.T) {
	cases := []struct {
		name     string
		kind     work.CustomFieldKind
		currency string
		detail   string
	}{
		{"money with its currency", work.CustomFieldMoney, "EUR", ""},
		{"money without", work.CustomFieldMoney, " ", "fields.currency_required"},
		{"money in lower case", work.CustomFieldMoney, "eur", "fields.currency_malformed"},
		{"money with four letters", work.CustomFieldMoney, "EURO", "fields.currency_malformed"},
		{"a number with a currency", work.CustomFieldNumber, "EUR", "fields.currency_not_applicable"},
		{"a duration without", work.CustomFieldDuration, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			built, err := typedDefinition(t, c.kind, c.currency)
			if c.detail != "" {
				if detail := shared.AsError(err).DetailCode; detail != c.detail {
					t.Fatalf("detail %q, want %s", detail, c.detail)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if built.Currency != strings.TrimSpace(c.currency) {
				t.Errorf("the currency is %q", built.Currency)
			}
		})
	}
}

// UC-WRK-12 checks 1 and 4: each of the suite's kinds accepts its own shape, normalised, and refuses
// every other with a code naming the field.
func TestTheSuitesKindsAcceptTheirOwnShape(t *testing.T) {
	account := "0192f000-0000-7000-8000-0000000000a1"
	other := "0192f000-0000-7000-8000-0000000000a2"
	cases := []struct {
		name   string
		kind   work.CustomFieldKind
		value  any
		want   any
		detail string
	}{
		{"money", work.CustomFieldMoney, 12.5, 12.5, ""},
		{"money to four places", work.CustomFieldMoney, 0.1234, 0.1234, ""},
		{"money finer than four places", work.CustomFieldMoney, 0.12345, nil, "fields.value_too_precise"},
		{"money as a string", work.CustomFieldMoney, "12.50", nil, "fields.value_type_mismatch"},
		{"money too large", work.CustomFieldMoney, 1e12, nil, "fields.value_too_large"},
		{"a duration", work.CustomFieldDuration, 90.0, 90.0, ""},
		{"a fractional duration", work.CustomFieldDuration, 1.5, nil, "fields.value_not_whole_minutes"},
		{"a negative duration", work.CustomFieldDuration, -5.0, nil, "fields.value_out_of_range"},
		{"a reference", work.CustomFieldReference, account, account, ""},
		{"a reference that is no id", work.CustomFieldReference, "nope", nil, "fields.value_not_an_entry"},
		{"people", work.CustomFieldUserSet, []any{account, other, account}, []any{account, other}, ""},
		{"people as one", work.CustomFieldUserSet, account, nil, "fields.value_type_mismatch"},
		{"people with a stranger", work.CustomFieldUserSet, []any{"x"}, nil, "fields.value_not_an_account"},
		{"no people", work.CustomFieldUserSet, []any{}, nil, "fields.value_empty"},
		{"rich text", work.CustomFieldRichText, " **bold** ", "**bold**", ""},
		{"rich text too long", work.CustomFieldRichText,
			strings.Repeat("a", work.MaxCustomFieldRichTextLength+1), nil, "fields.value_too_long"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			currency := ""
			if c.kind == work.CustomFieldMoney {
				currency = "EUR"
			}
			field, err := typedDefinition(t, c.kind, currency)
			if err != nil {
				t.Fatalf("building: %v", err)
			}
			got, err := field.ValidateValue(c.value, text.Composing{})
			if c.detail != "" {
				if detail := shared.AsError(err).DetailCode; detail != c.detail {
					t.Fatalf("detail %q, want %s", detail, c.detail)
				}
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

// The accounts a person field names are what the application holds against the entry's readers.
func TestAPersonFieldNamesItsAccounts(t *testing.T) {
	account := "0192f000-0000-7000-8000-0000000000a1"
	set, _ := typedDefinition(t, work.CustomFieldUserSet, "")
	if got := set.Accounts([]any{account}); len(got) != 1 || got[0].String() != account {
		t.Errorf("a set names %v", got)
	}
	single, _ := typedDefinition(t, work.CustomFieldUser, "")
	if got := single.Accounts(account); len(got) != 1 {
		t.Errorf("a person names %v", got)
	}
	reference, _ := typedDefinition(t, work.CustomFieldReference, "")
	if got := reference.Accounts(account); len(got) != 0 {
		t.Errorf("a reference names accounts %v", got)
	}
}

// UC-WRK-12: a CALCULATED field carries a formula and nothing else does; nobody fills it in, clears
// it or requires it, and its value is a finite number to four places, or none.
func TestACalculatedFieldIsTheServersAlone(t *testing.T) {
	build := func(kind work.CustomFieldKind, formula string, required bool) (work.CustomFieldDefinition, error) {
		return work.NewCustomFieldDefinition(work.NewCustomFieldInput{
			ID: fieldID, TenantID: fieldTenant, Key: "cost", Kind: kind, Formula: formula, IsRequired: required,
			AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow, Text: text.Composing{},
		})
	}
	calculated, err := build(work.CustomFieldCalculated, "  entry.custom_fields.hours * 2.0 ", false)
	if err != nil || calculated.Formula != "entry.custom_fields.hours * 2.0" {
		t.Fatalf("a calculated field was refused: %+v, %v", calculated, err)
	}
	refusals := map[string]struct {
		kind     work.CustomFieldKind
		formula  string
		required bool
	}{
		"fields.formula_required":       {work.CustomFieldCalculated, " ", false},
		"fields.formula_not_applicable": {work.CustomFieldNumber, "1.0", false},
		"fields.formula_invalid":        {work.CustomFieldCalculated, strings.Repeat("1", work.MaxCustomFieldFormulaLength+1), false},
		"fields.calculated":             {work.CustomFieldCalculated, "1.0", true},
	}
	for code, in := range refusals {
		if _, err := build(in.kind, in.formula, in.required); shared.AsError(err) == nil || shared.AsError(err).DetailCode != code {
			t.Errorf("%s: answered %v", code, err)
		}
	}
	for _, value := range []any{12.0, nil} {
		if _, err := calculated.ValidateValue(value, text.Composing{}); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "fields.calculated" {
			t.Errorf("writing %v to a calculated field answered %v", value, err)
		}
	}
	required := true
	if _, _, err := calculated.Updated(work.CustomFieldAttributes{IsRequired: &required}, text.Composing{}, fieldNow); err == nil {
		t.Error("a calculated field was made required")
	}
	if got := work.CalculatedValue(10.0/3.0, true); got != 3.3333 {
		t.Errorf("10/3 is stored as %v", got)
	}
	if work.CalculatedValue(1, false) != nil || work.CalculatedValue(math.Inf(1), true) != nil {
		t.Error("an unanswered or infinite result was stored")
	}
}
