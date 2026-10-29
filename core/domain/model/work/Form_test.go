// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func fieldCodes(err error) []string {
	var coded *shared.Error
	if !errors.As(err, &coded) {
		return nil
	}
	codes := []string{}
	for _, field := range coded.Fields {
		codes = append(codes, field.Path+" "+field.Code)
	}
	return codes
}

// A form as designed is held to its invariants: keys, sections, a title never hidden, a channel
// that only narrows (model.md §6).
func TestNewFormInvariants(t *testing.T) {
	severity := work.FormField{Key: "custom_fields.severity", Required: true}
	cases := []struct {
		name string
		form work.Form
		want string
	}{
		{"empty", work.Form{}, ""},
		{"designed", work.Form{
			Sections: []work.FormSection{{Key: "triage", Name: "Triage"}},
			Fields: []work.FormField{
				{Key: "title"}, {Key: "custom_fields.severity", Section: "triage", Required: true, Default: "low"},
				{Key: "due_date", RequiredWhen: `entry.custom_fields.severity == "high"`},
			},
			Validations: []work.FormValidation{{Key: "short", Field: "title", Holds: "size(entry.title) < 80", Message: "Keep it short"}},
			Channels: []work.FormChannelRule{{Channel: work.ChannelMail, Hidden: []string{"custom_fields.severity"},
				Presets: map[string]any{"custom_fields.severity": "low"}}},
		}, ""},
		{"unknown key", work.Form{Fields: []work.FormField{{Key: "colour"}}}, "forms.field_key_unknown"},
		{"twice", work.Form{Fields: []work.FormField{{Key: "notes"}, {Key: "notes"}}}, "forms.key_taken"},
		{"no such section", work.Form{Fields: []work.FormField{{Key: "notes", Section: "x"}}}, "forms.section_unknown"},
		{"title hidden", work.Form{Fields: []work.FormField{{Key: "title", HiddenWhen: "true"}}}, "forms.title_not_hideable"},
		{"default on a core field", work.Form{Fields: []work.FormField{{Key: "notes", Default: "x"}}}, "forms.default_not_custom"},
		{"section without a name", work.Form{Sections: []work.FormSection{{Key: "a"}}}, "forms.section_name_empty"},
		{"validation of no field", work.Form{Validations: []work.FormValidation{{Key: "v", Field: "notes", Holds: "true"}}}, "forms.field_not_in_form"},
		{"validation empty", work.Form{Validations: []work.FormValidation{{Key: "v"}}}, "forms.validation_empty"},
		{"unknown channel", work.Form{Channels: []work.FormChannelRule{{Channel: "PORTAL2"}}}, "forms.channel_unknown"},
		{"channel adds a field", work.Form{Channels: []work.FormChannelRule{{Channel: work.ChannelMail, Hidden: []string{"notes"}}}}, "forms.field_not_in_form"},
		{"channel hides a required field and fills nothing", work.Form{
			Fields:   []work.FormField{severity},
			Channels: []work.FormChannelRule{{Channel: work.ChannelMail, Hidden: []string{"custom_fields.severity"}}},
		}, "forms.required_hidden_unfilled"},
		{"channel presets a field outside the form", work.Form{
			Channels: []work.FormChannelRule{{Channel: work.ChannelMail, Presets: map[string]any{"custom_fields.x": 1}}},
		}, "forms.field_not_in_form"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := work.NewForm(c.form, text.Composing{})
			if got := detailOf(err); got != c.want {
				t.Fatalf("want %q, got %q (%v)", c.want, got, err)
			}
		})
	}
}

// The state puts the answered conditions together: a channel's hidden field is hidden, a hidden
// field is never required, a validation of a hidden field is not asked.
func TestFormState(t *testing.T) {
	form := work.Form{
		Fields: []work.FormField{
			{Key: "custom_fields.severity", Required: true},
			{Key: "due_date", RequiredWhen: "high"},
			{Key: "notes", ReadOnlyWhen: "locked", HiddenWhen: "secret"},
		},
		Validations: []work.FormValidation{{Key: "v", Field: "notes", Holds: "fails"}, {Key: "w", Holds: "fails"}},
		Channels: []work.FormChannelRule{{Channel: work.ChannelMail, Closed: true,
			Hidden: []string{"custom_fields.severity"}, Presets: map[string]any{"custom_fields.severity": "low"}}},
	}
	answers := map[string]bool{"high": true, "locked": true, "secret": true, "fails": false}
	answer := func(expression string) (bool, error) { return answers[expression], nil }

	app, err := form.State(work.ChannelApp, answer)
	if err != nil {
		t.Fatal(err)
	}
	if !app.Fields["custom_fields.severity"].Required || !app.Fields["due_date"].Required || app.Closed {
		t.Fatalf("app: %+v", app)
	}
	if notes := app.Fields["notes"]; !notes.Hidden || !notes.ReadOnly || notes.Required {
		t.Fatalf("notes: %+v", notes)
	}
	if len(app.Failed) != 1 || app.Failed[0] != "w" {
		t.Fatalf("a validation of a hidden field was asked: %v", app.Failed)
	}
	mail, _ := form.State(work.ChannelMail, answer)
	if severity := mail.Fields["custom_fields.severity"]; !severity.Hidden || severity.Required || !mail.Closed {
		t.Fatalf("mail: %+v", mail)
	}
	if err := work.RefuseClosedChannel(mail, "bug", work.ChannelMail); detailOf(err) != "forms.channel_closed" {
		t.Fatalf("closed channel: %v", err)
	}
	starting := form.Starting(work.ChannelMail, map[string]any{})
	if starting["severity"] != "low" {
		t.Fatalf("preset: %v", starting)
	}
}

// A write may not move an entry further from its form; what an old entry already lacked is not
// held against an unrelated write.
func TestCheckFormWrite(t *testing.T) {
	form := work.Form{Fields: []work.FormField{
		{Key: "custom_fields.severity", Required: true}, {Key: "notes"}, {Key: "title"},
	}, Validations: []work.FormValidation{{Key: "v", Holds: "x"}}}
	required := work.FormState{Fields: map[string]work.FormFieldState{"custom_fields.severity": {Required: true}}}
	readOnly := work.FormState{Fields: map[string]work.FormFieldState{"notes": {ReadOnly: true}}}
	hidden := work.FormState{Fields: map[string]work.FormFieldState{"notes": {Hidden: true}}}
	failing := work.FormState{Failed: []string{"v"}}
	empty := work.WorkItem{Title: "a"}
	filled := work.WorkItem{Title: "a", CustomFields: map[string]any{"severity": "high"}}
	noted := work.WorkItem{Title: "a", Notes: "n"}

	cases := []struct {
		name         string
		before       *work.WorkItem
		state, after work.FormState
		item         work.WorkItem
		want         []string
	}{
		{"create without a required field", nil, work.FormState{}, required, empty, []string{"/custom_fields/severity fields.value_required"}},
		{"create with it", nil, work.FormState{}, required, filled, nil},
		{"edit clears it", &filled, required, required, empty, []string{"/custom_fields/severity fields.value_required"}},
		{"edit of an entry that already lacked it", &empty, required, required, empty, nil},
		{"read-only changed", &empty, readOnly, readOnly, noted, []string{"/notes forms.field_read_only"}},
		{"hidden changed", &empty, hidden, hidden, noted, []string{"/notes forms.field_hidden"}},
		{"read-only untouched", &noted, readOnly, readOnly, noted, nil},
		{"validation newly failing", &empty, work.FormState{}, failing, empty, []string{" forms.validation_failed"}},
		{"validation failing before", &empty, failing, failing, empty, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := work.CheckFormWrite(form, c.before, c.state, c.item, c.after)
			got := fieldCodes(err)
			if len(got) != len(c.want) {
				t.Fatalf("want %v, got %v", c.want, got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("want %v, got %v", c.want, got)
				}
			}
			if err != nil && detailOf(err) != "forms.refused" {
				t.Fatalf("detail %q", detailOf(err))
			}
		})
	}
}

// A form's `step` makes a field required or a validation fail only while a step is taken; the step
// is refused for that, and not for what the form asks regardless of it (UC-FLW-04 check 2).
func TestAStepIsHeldToWhatTheFormAsksOfIt(t *testing.T) {
	form := work.Form{
		Fields:      []work.FormField{{Key: "custom_fields.resolution"}, {Key: "custom_fields.owner"}},
		Validations: []work.FormValidation{{Key: "amount_positive"}},
	}
	always := work.FormState{Fields: map[string]work.FormFieldState{"custom_fields.owner": {Required: true}}}
	stepping := func(failed ...string) work.FormState {
		return work.FormState{Fields: map[string]work.FormFieldState{
			"custom_fields.owner": {Required: true}, "custom_fields.resolution": {Required: true},
		}, Failed: failed}
	}
	empty := work.WorkItem{Title: "a"}
	resolved := work.WorkItem{Title: "a", CustomFields: map[string]any{"resolution": "fixed"}}

	tests := []struct {
		name    string
		item    work.WorkItem
		with    work.FormState
		refused []string
	}{
		{"a field the step requires is empty", empty, stepping(), []string{"/custom_fields/resolution"}},
		{"the field is given", resolved, stepping(), nil},
		{"a validation fails only for the step", resolved, stepping("amount_positive"), []string{""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := work.CheckFormStep(form, test.item, always, test.with)
			if test.refused == nil {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || len(refusal.Fields) != len(test.refused) {
				t.Fatalf("err = %v, want the fields %v", err, test.refused)
			}
			for i, path := range test.refused {
				if refusal.Fields[i].Path != path {
					t.Errorf("field %d = %q, want %q", i, refusal.Fields[i].Path, path)
				}
			}
		})
	}
}
