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

// UC-WRK-12 checks 1 and 4: the kinds a register and a tracker add - a moment, a group, a two-level
// choice, an address, an attachment, labels and a state - accept their own shape, normalised, and
// refuse every other with a code naming the field.
func TestTheRegisterKindsAcceptTheirOwnShape(t *testing.T) {
	group := "0192f000-0000-7000-8000-0000000000b1"
	cases := []struct {
		name    string
		kind    work.CustomFieldKind
		options []string
		value   any
		want    any
		detail  string
	}{
		{"a moment in UTC", work.CustomFieldDateTime, nil, "2026-10-31T10:15:30+01:00", "2026-10-31T09:15:30Z", ""},
		{"a moment's fraction is dropped", work.CustomFieldDateTime, nil, "2026-10-31T09:15:30.987Z", "2026-10-31T09:15:30Z", ""},
		{"a day is no moment", work.CustomFieldDateTime, nil, "2026-10-31", nil, "fields.value_not_a_moment"},
		{"a moment as a number", work.CustomFieldDateTime, nil, 1.0, nil, "fields.value_type_mismatch"},
		{"a group", work.CustomFieldGroup, nil, group, group, ""},
		{"a group that is no id", work.CustomFieldGroup, nil, "admins", nil, "fields.value_not_a_group"},
		{"a parent", work.CustomFieldCascadingSelect, []string{"Hardware", "Hardware/Laptop"}, "Hardware", "Hardware", ""},
		{"a child", work.CustomFieldCascadingSelect, []string{"Hardware", "Hardware/Laptop"}, "Hardware/Laptop", "Hardware/Laptop", ""},
		{"a child not offered", work.CustomFieldCascadingSelect, []string{"Hardware", "Hardware/Laptop"}, "Hardware/Phone", nil, "fields.value_not_an_option"},
		{"an IPv4 address", work.CustomFieldIPAddress, nil, " 192.168.0.10 ", "192.168.0.10", ""},
		{"an IPv6 address in its canonical spelling", work.CustomFieldIPAddress, nil, "2001:DB8:0:0:0:0:0:1", "2001:db8::1", ""},
		{"a network is no address", work.CustomFieldIPAddress, nil, "10.0.0.0/8", nil, "fields.value_not_an_address"},
		{"an address with a zone", work.CustomFieldIPAddress, nil, "fe80::1%eth0", nil, "fields.value_not_an_address"},
		{"a host name is no address", work.CustomFieldIPAddress, nil, "intranet", nil, "fields.value_not_an_address"},
		{"a file", work.CustomFieldFile, nil, group, group, ""},
		{"a file that is no id", work.CustomFieldFile, nil, "report.pdf", nil, "fields.value_not_an_attachment"},
		{"labels in lower case, once each", work.CustomFieldLabel, nil, []any{"Backend", "v2.1", "backend"}, []any{"backend", "v2.1"}, ""},
		{"a label with a space", work.CustomFieldLabel, nil, []any{"two words"}, nil, "fields.value_not_a_label"},
		{"a label too long", work.CustomFieldLabel, nil, []any{strings.Repeat("a", work.MaxCustomFieldLabelLength+1)}, nil, "fields.value_not_a_label"},
		{"too many labels", work.CustomFieldLabel, nil, manyLabels(work.MaxCustomFieldLabels + 1), nil, "fields.value_too_many"},
		{"no labels", work.CustomFieldLabel, nil, []any{}, nil, "fields.value_empty"},
		{"a label alone", work.CustomFieldLabel, nil, "backend", nil, "fields.value_type_mismatch"},
		{"a state", work.CustomFieldStatus, []string{"Planned", "In use", "Retired"}, "In use", "In use", ""},
		{"a state not offered", work.CustomFieldStatus, []string{"Planned", "In use", "Retired"}, "Lost", nil, "fields.value_not_an_option"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			field, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
				ID: fieldID, TenantID: fieldTenant, CollectionID: fieldColl,
				Key: "priority", Kind: c.kind, Options: c.options,
				AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow, Text: text.Composing{},
			})
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

func manyLabels(n int) []any {
	labels := make([]any, n)
	for i := range labels {
		labels[i] = "l" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	return labels
}

// UC-WRK-12 check 2: a two-level choice's options are paths of one or two parts, each child under a
// parent the list offers; a state takes options as a pick does; the others take none.
func TestACascadingChoiceOffersChildrenUnderTheirParents(t *testing.T) {
	cases := []struct {
		name    string
		kind    work.CustomFieldKind
		options []string
		detail  string
	}{
		{"parents and children", work.CustomFieldCascadingSelect, []string{"Hardware", "Hardware/Laptop", "Software"}, ""},
		{"a child without its parent", work.CustomFieldCascadingSelect, []string{"Hardware/Laptop"}, "fields.option_without_parent"},
		{"three levels", work.CustomFieldCascadingSelect, []string{"A", "A/B", "A/B/C"}, "fields.option_path_malformed"},
		{"an empty part", work.CustomFieldCascadingSelect, []string{"A", "A/"}, "fields.option_path_malformed"},
		{"no options", work.CustomFieldCascadingSelect, nil, "fields.options_required"},
		{"a state without states", work.CustomFieldStatus, nil, "fields.options_required"},
		{"a label field with options", work.CustomFieldLabel, []string{"a"}, "fields.options_not_applicable"},
		{"a moment with options", work.CustomFieldDateTime, []string{"a"}, "fields.options_not_applicable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
				ID: fieldID, TenantID: fieldTenant, Key: "kind", Kind: c.kind, Options: c.options,
				AppliesTo: []work.ItemType{work.ItemTask}, Now: fieldNow, Text: text.Composing{},
			})
			if c.detail == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if detail := shared.AsError(err).DetailCode; detail != c.detail {
				t.Fatalf("detail %q, want %s", detail, c.detail)
			}
		})
	}
}

// The group and the attachment a value names are what the application holds against the
// workspace and the entry.
func TestAGroupAndAFileFieldNameTheirTargets(t *testing.T) {
	id := "0192f000-0000-7000-8000-0000000000b1"
	group, _ := typedDefinition(t, work.CustomFieldGroup, "")
	if got, ok := group.GroupOf(id); !ok || got.String() != id {
		t.Errorf("a group field names %v, %v", got, ok)
	}
	file, _ := typedDefinition(t, work.CustomFieldFile, "")
	if got, ok := file.AttachmentOf(id); !ok || got.String() != id {
		t.Errorf("a file field names %v, %v", got, ok)
	}
	if _, ok := file.GroupOf(id); ok {
		t.Error("a file field names a group")
	}
	if _, ok := group.AttachmentOf(nil); ok {
		t.Error("a cleared value names an attachment")
	}
}
