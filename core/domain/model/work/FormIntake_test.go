// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A form that newly requires a value nobody at an unattended door fills - mail, a rule, an import -
// leaves that door refusing every creation of the type; the plan names each such door and field.
func TestIntakeGapsNameTheDoorsANewlyRequiredFieldCloses(t *testing.T) {
	severity := "custom_fields.severity"
	required := work.FormField{Key: severity, Required: true}
	cases := map[string]struct {
		before, after work.Form
		want          []work.IntakeGap
	}{
		"newly required, nothing fills it": {
			before: work.Form{},
			after:  work.Form{Fields: []work.FormField{required}},
			want: []work.IntakeGap{
				{Channel: work.ChannelRule, Fields: []string{severity}},
				{Channel: work.ChannelImport, Fields: []string{severity}},
				{Channel: work.ChannelMail, Fields: []string{severity}},
			},
		},
		"required before as well": {
			before: work.Form{Fields: []work.FormField{required}},
			after:  work.Form{Fields: []work.FormField{required}},
		},
		"a default fills it": {
			after: work.Form{Fields: []work.FormField{{Key: severity, Required: true, Default: "low"}}},
		},
		"only under a condition": {
			after: work.Form{Fields: []work.FormField{{Key: severity, RequiredWhen: "entry.priority == 1"}}},
		},
		"mail presets it, mail closed elsewhere": {
			after: work.Form{
				Fields: []work.FormField{required},
				Channels: []work.FormChannelRule{
					{Channel: work.ChannelMail, Presets: map[string]any{severity: "low"}},
					{Channel: work.ChannelImport, Closed: true},
				},
			},
			want: []work.IntakeGap{{Channel: work.ChannelRule, Fields: []string{severity}}},
		},
		"the title every door sends, and mail's notes": {
			after: work.Form{Fields: []work.FormField{
				{Key: work.FormKeyTitle, Required: true}, {Key: work.FormKeyNotes, Required: true},
			}},
			want: []work.IntakeGap{
				{Channel: work.ChannelRule, Fields: []string{work.FormKeyNotes}},
				{Channel: work.ChannelImport, Fields: []string{work.FormKeyNotes}},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := work.IntakeGaps(tc.before, tc.after); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("gaps %+v, want %+v", got, tc.want)
			}
		})
	}
}
