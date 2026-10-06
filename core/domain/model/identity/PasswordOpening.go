// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// An operator opens the password for one workspace (ADR-0078 §3, SC-34).
//
// For a provider that is switched on but broken - unreachable, or admitting nobody - nothing inside
// the workspace can help: the administrators who could switch the password back on cannot sign in to
// do it. The operator can open it for that one workspace, for a limited time, on somebody's request.
// It is ADR-0076 §4's fallback with a person's decision as its cause: every account that holds a
// password signs in with it, under the workspace's own rules and second factor, whatever the
// workspace's switch and any installation lock say. It reads and changes nothing of the workspace's
// content (P-01) - it is a fact about the way in, held on the tenant row beside the lifecycle.

// The bounds ADR-0078 §3 sets: a day unless the operator says otherwise, and never more than a week.
// Counted in hours, because that is the unit an operator answering a ticket thinks in.
const (
	PasswordOpeningDefaultHours = 24
	PasswordOpeningMaximumHours = 7 * 24
)

// The lengths the schema holds (migration 0118). A ticket reference and a sentence, not a report.
const (
	maxOpeningRequester = 200
	maxOpeningReason    = 500
)

// PasswordOpening is the opening as it stands: when it ends, who asked for it and why. The zero value
// is no opening.
type PasswordOpening struct {
	Until time.Time
	// Requester is free text: a ticket reference where the operator can give one, a name where they
	// cannot. Personal data either way, so it lives on the row only while the opening does
	// (docs/privacy/data-catalog.md).
	Requester string
	Reason    string
}

// NewOpening checks what the operator entered and dates the end of a PasswordOpening.
//
// The hours are counted from `now`; a second opening is a new one rather than an extension, which is
// what an operator who answers a second request means. (Named without "password": CodeQL's
// sensitive-data heuristic reads any value from an identifier spelled so as a password, and traced
// this one - a moment, a ticket reference and a sentence - into the trail's fingerprint as one.)
func NewOpening(
	hours int, requester, reason string, now time.Time, form text.Normalizer,
) (PasswordOpening, error) {
	if hours < 1 || hours > PasswordOpeningMaximumHours {
		return PasswordOpening{}, shared.ErrValidation.
			WithDetail("admin.password_opening_hours").
			WithParams(map[string]string{"maximum": itoa(PasswordOpeningMaximumHours)}).
			WithFields(shared.FieldError{Path: "/hours", Code: "admin.password_opening_hours"})
	}
	who, err := openingText(requester, form)
	if err != nil {
		return PasswordOpening{}, err
	}
	if err := boundOpeningText(who, "/requester", maxOpeningRequester,
		"admin.password_opening_requester_required", "admin.password_opening_requester_too_long"); err != nil {
		return PasswordOpening{}, err
	}
	why, err := openingText(reason, form)
	if err != nil {
		return PasswordOpening{}, err
	}
	if err := boundOpeningText(why, "/reason", maxOpeningReason,
		"admin.password_opening_reason_required", "admin.password_opening_reason_too_long"); err != nil {
		return PasswordOpening{}, err
	}
	return PasswordOpening{
		Until:     now.Add(time.Duration(hours) * time.Hour).UTC(),
		Requester: who,
		Reason:    why,
	}, nil
}

// openingText trims one of the two texts and brings it to normal form C (M-07). Its own function, with
// no message code among its arguments, because the text it answers is recorded: a call handed a code
// spelled "password_…" is what CodeQL's heuristic reads as a password source.
func openingText(raw string, form text.Normalizer) (string, error) {
	return shared.NFC(strings.TrimSpace(raw), form)
}

// boundOpeningText refuses an empty or overlong text at its field. Empty is refused: an opening
// nobody asked for, or that nobody can say the reason of, is not one an operator should be able to
// make without noticing.
func boundOpeningText(value, path string, maximum int, required, tooLong string) error {
	switch {
	case value == "":
		return shared.ErrValidation.
			WithDetail(required).
			WithFields(shared.FieldError{Path: path, Code: required})
	case utf8.RuneCountInString(value) > maximum:
		params := map[string]string{"maximum": itoa(maximum)}
		return shared.ErrValidation.
			WithDetail(tooLong).
			WithParams(params).
			WithFields(shared.FieldError{Path: path, Code: tooLong, Params: params})
	}
	return nil
}

// InForce answers whether the opening stands at `now`. Its end is honoured here, where it is read:
// past it the opening is over whether or not anything has cleared the row yet, so no job has to run
// for the password to close.
func (o PasswordOpening) InForce(now time.Time) bool {
	return !o.Until.IsZero() && now.Before(o.Until)
}
