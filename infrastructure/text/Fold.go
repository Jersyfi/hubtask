// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package text

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Folded is a name as the unique name indexes compare it (`lower(imm_unaccent(name))`): trimmed,
// lower case, its accents taken off. For an adapter that has to know before writing whether the
// database would take two names for one - the import meeting a collection's own columns and labels.
func Folded(name string) string {
	var folded strings.Builder
	for _, r := range norm.NFD.String(strings.TrimSpace(name)) {
		if !unicode.Is(unicode.Mn, r) {
			folded.WriteRune(unicode.ToLower(r))
		}
	}
	return folded.String()
}
