// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// MaxReplyBodyLength bounds a reply template's text: a comment's bound, since a rendered template
// becomes one.
const MaxReplyBodyLength = 20000

// ReplyVariables are the placeholders a reply template may name, written `{{entry.title}}`. A
// closed set, so a template that names anything else is refused when it is written rather than
// answering a half-filled reply.
var ReplyVariables = []string{"entry.title", "entry.id", "requester.name", "agent.name"}

var placeholder = regexp.MustCompile(`\{\{\s*([a-z_]+(?:\.[a-z_]+)*)\s*\}\}`)

// ReplyTemplate is a canned reply under a key (UC-SVC-06): text a handler sends often, with
// placeholders the entry fills.
type ReplyTemplate struct {
	ID        shared.ID
	TenantID  shared.ID
	Key       string
	Name      string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// NewReplyTemplateInput is what defining a reply template takes.
type NewReplyTemplateInput struct {
	ID, TenantID    shared.ID
	Key, Name, Body string
	Now             time.Time
	Text            text.Normalizer
}

// NewReplyTemplate checks a reply template: a key, a name on one line, a body within the
// comment's bound that names only the known placeholders.
func NewReplyTemplate(in NewReplyTemplateInput) (ReplyTemplate, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return ReplyTemplate{}, shared.ErrInternal.WithDetail("reply_templates.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !keyPattern.MatchString(key) {
		return ReplyTemplate{}, invalid("/key", "reply_templates.key_malformed")
	}
	name, err := checkName(in.Name, in.Text, "reply_templates")
	if err != nil {
		return ReplyTemplate{}, err
	}
	body := strings.TrimSpace(in.Body)
	if in.Text != nil {
		body = in.Text.NFC(body)
	}
	switch {
	case body == "":
		return ReplyTemplate{}, invalid("/body", "reply_templates.body_required")
	case utf8.RuneCountInString(body) > MaxReplyBodyLength:
		return ReplyTemplate{}, invalid("/body", "reply_templates.body_too_long")
	}
	for _, match := range placeholder.FindAllStringSubmatch(body, -1) {
		if !slices.Contains(ReplyVariables, match[1]) {
			return ReplyTemplate{}, shared.ErrValidation.WithDetail("reply_templates.variable_unknown").
				WithParams(map[string]string{"variable": match[1]}).
				WithFields(shared.FieldError{Path: "/body", Code: "reply_templates.variable_unknown"})
		}
	}
	return ReplyTemplate{
		ID: in.ID, TenantID: in.TenantID, Key: key, Name: name, Body: body,
		CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// Change answers the template with a new name and body, its key kept.
func (t ReplyTemplate) Change(name, body string, now time.Time, normalizer text.Normalizer) (ReplyTemplate, error) {
	changed, err := NewReplyTemplate(NewReplyTemplateInput{
		ID: t.ID, TenantID: t.TenantID, Key: t.Key, Name: name, Body: body, Now: now, Text: normalizer,
	})
	if err != nil {
		return ReplyTemplate{}, err
	}
	changed.CreatedAt, changed.Version = t.CreatedAt, t.Version
	return changed, nil
}

// Render fills the placeholders from values; one without a value becomes empty, never the
// placeholder itself, so a requester never reads `{{…}}`.
func (t ReplyTemplate) Render(values map[string]string) string {
	return placeholder.ReplaceAllStringFunc(t.Body, func(found string) string {
		return values[placeholder.FindStringSubmatch(found)[1]]
	})
}
