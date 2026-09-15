// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "github.com/Jersyfi/hubtask/core/port/text"

// NFC brings the input to normal form C through the port (i18n-l10n.md §5, M-07), and does
// nothing else: whether a text is trimmed is each kind's own rule - a title is, a comment is
// prose and is stored as sent - and this is the one thing every kind shares.
//
// Every constructor that stores user text calls this before it bounds and checks the value, so
// that the length is counted, the uniqueness index compares, and the search document indexes
// the one spelling a person sees. It lives here rather than in each bounded context for the
// reason LanguageTag does: a title is work's, a display name is identity's, and the normal form
// of text is neither's.
//
// Without a port, text that is not ASCII is refused rather than stored as it arrived - fail
// closed, exactly as an email's domain is without its encoder: a caller that forgot to wire
// the port cannot create the second label this exists to prevent. ASCII cannot be in any other
// form than its own, so a caller that never sees anything else loses nothing. The refusal is an
// internal error and not a validation one: nothing the client sent is wrong.
func NFC(raw string, form text.Normalizer) (string, error) {
	if form == nil {
		if !isASCII(raw) {
			return "", ErrInternal.WithDetail("text.normalizer_missing")
		}
		return raw, nil
	}
	return form.NFC(raw), nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
