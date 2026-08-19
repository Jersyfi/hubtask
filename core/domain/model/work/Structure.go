// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The structure of a collection: the buckets its items are arranged in, and the labels they are
// tagged with (domain-model.md §3.5). Both belong to exactly one collection, both are named, and
// both are unique within it - so what they share is written once, here.

// MaxStructureNameLength counts code points rather than bytes, for the reason
// MaxContainerNameLength does: a limit in bytes draws a line a user cannot see. 120 is what the
// column allows (db/schema.sql, bucket and label).
const MaxStructureNameLength = 120

// nameCodes are the three message codes one entity's name rule reports with.
//
// Passed in rather than built from a prefix, so that every code in this package stays a literal a
// person can grep for. A code assembled at runtime is a code that is not in the catalogue until
// somebody runs the line that builds it (i18n-l10n.md §3).
type nameCodes struct{ empty, tooLong, malformed string }

// structureName trims and checks the name of a bucket or a label.
//
// Trimmed here rather than in the adapter, because the trimmed form is what the uniqueness index
// compares: " Doing" and "Doing" are the same name to a person, and a check that disagrees with a
// person is a bug report waiting.
func structureName(raw string, codes nameCodes) (string, error) {
	name := strings.TrimSpace(raw)

	switch {
	case name == "":
		return "", shared.ErrValidation.
			WithDetail(codes.empty).
			WithFields(shared.FieldError{Path: "/name", Code: codes.empty})

	case utf8.RuneCountInString(name) > MaxStructureNameLength:
		return "", shared.ErrValidation.
			WithDetail(codes.tooLong).
			WithParams(map[string]string{"maximum": "120"}).
			WithFields(shared.FieldError{Path: "/name", Code: codes.tooLong})

	case hasControlCharacter(name):
		// One line, like a container's name and for the same reason: a newline survives every
		// layer and then breaks the one that renders it - a CSV export, a board column header.
		return "", shared.ErrValidation.
			WithDetail(codes.malformed).
			WithFields(shared.FieldError{Path: "/name", Code: codes.malformed})
	}
	return name, nil
}

// colorToken trims and checks a colour token.
//
// A token rather than a colour value, so that a client renders it in its own palette and a dark
// theme is a client decision rather than a stored one (domain-model.md §3.5). The empty string is
// "not set"; whether that is allowed is the caller's question, because a bucket may have no colour
// and a label may not.
func colorToken(raw string, code string) (string, error) {
	token := strings.TrimSpace(raw)
	if token != "" && hasControlCharacter(token) {
		return "", shared.ErrValidation.
			WithDetail(code).
			WithFields(shared.FieldError{Path: "/color_token", Code: code})
	}
	return token, nil
}

// Field names of a collection's structure, as the API spells them. They travel into the event's
// change set and into the change log, where a client matches them against the members it sent - so
// they are written once here rather than as a literal at each place that has to agree.
//
// FieldName, FieldDescription, FieldColorToken and FieldOrderKey are shared with Container and
// declared there.
const (
	FieldWipLimit     = "wip_limit"
	FieldIsDoneBucket = "is_done_bucket"
	FieldDeletedAt    = "deleted_at"
)
