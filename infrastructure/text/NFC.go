// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package text

import (
	"golang.org/x/text/unicode/norm"

	port "github.com/Jersyfi/hubtask/core/port/text"
)

// Forms brings text to normal form C (i18n-l10n.md §5). NFC rather than NFKC: compatibility
// decomposition would rewrite what a person typed - `ﬁ` into `fi`, `²` into `2`, full-width
// letters into ASCII - and a title is content, not an identifier. Canonical composition changes
// only the encoding of a character, never which character it is.
type Forms struct{}

var _ port.Normalizer = Forms{}

// NFC is the port's method. norm.NFC.String answers the input unchanged, without allocating,
// when it is already in normal form - which is nearly always.
func (Forms) NFC(text string) string {
	return norm.NFC.String(text)
}
