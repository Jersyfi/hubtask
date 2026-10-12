// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "time"

// Terms of use, agreed and agreed again (UC-ID-19, identity.md §4.2 and §12).
//
// Hubtask holds no text of the terms: the terms are the link the workspace or the installation set
// (LegalLinks.TermsURL) and, where they gave one, a version beside it. What a person agrees to is
// that pair, so an agreement is current exactly while the stored link and version equal the ones in
// force - changing either asks everybody once more, which the settings screen says beside both.

// TermsAgreement is a person's latest agreement: the terms' link and version, and when. The zero
// value is "never agreed".
type TermsAgreement struct {
	URL      string
	Version  string
	AgreedAt time.Time
}

// Agreed reports whether the person ever agreed to anything.
func (a TermsAgreement) Agreed() bool { return !a.AgreedAt.IsZero() }
