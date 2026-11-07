// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func replyInput(body string) NewReplyTemplateInput {
	return NewReplyTemplateInput{
		ID: shared.ID("11111111-1111-4111-8111-111111111111"), TenantID: shared.ID("22222222-2222-4222-8222-222222222222"),
		Key: "password_reset", Name: "Password reset", Body: body, Now: time.Unix(0, 0),
	}
}

func TestAReplyTemplateIsChecked(t *testing.T) {
	cases := []struct {
		name string
		edit func(*NewReplyTemplateInput)
		code string
	}{
		{"known placeholders", func(*NewReplyTemplateInput) {}, ""},
		{"a malformed key", func(in *NewReplyTemplateInput) { in.Key = "Reset" }, "reply_templates.key_malformed"},
		{"no name", func(in *NewReplyTemplateInput) { in.Name = " " }, "reply_templates.name_required"},
		{"no body", func(in *NewReplyTemplateInput) { in.Body = "  " }, "reply_templates.body_required"},
		{"too long", func(in *NewReplyTemplateInput) { in.Body = strings.Repeat("a", MaxReplyBodyLength+1) }, "reply_templates.body_too_long"},
		{"an unknown placeholder", func(in *NewReplyTemplateInput) { in.Body = "Hi {{entry.secret}}" }, "reply_templates.variable_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := replyInput("Hello {{requester.name}}, about {{ entry.title }}: {{agent.name}}")
			tc.edit(&in)
			_, err := NewReplyTemplate(in)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var de *shared.Error
			if !errors.As(err, &de) || de.DetailCode != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestARenderedReplyFillsEveryPlaceholder(t *testing.T) {
	template, err := NewReplyTemplate(replyInput("Hello {{requester.name}}, about {{ entry.title }} - {{agent.name}}"))
	if err != nil {
		t.Fatal(err)
	}
	got := template.Render(map[string]string{"requester.name": "Mia", "entry.title": "VPN"})
	if got != "Hello Mia, about VPN - " {
		t.Fatalf("rendered %q", got)
	}
}
