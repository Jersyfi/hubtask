// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxTenantReferenceLength bounds a purchase platform's own reference for a workspace.
const MaxTenantReferenceLength = 200

// ValidTenantReference checks a purchase platform's own reference for a workspace - its customer
// or subscription id (multi-tenancy.md §4.3). Opaque to Hubtask, and so taken as it is: no
// trimming, and white space at either end refused rather than removed, because two references
// that differ by a trailing blank are one mistake, not two customers.
func ValidTenantReference(reference string) error {
	if reference == "" || utf8.RuneCountInString(reference) > MaxTenantReferenceLength ||
		strings.TrimSpace(reference) != reference || strings.IndexFunc(reference, invisible) >= 0 {
		return shared.ErrValidation.WithDetail("admin.reference_invalid").
			WithFields(shared.FieldError{Path: "/external_reference", Code: "admin.reference_invalid"})
	}
	return nil
}

// invisible is a character a person cannot see in a reference: a control character, a line or
// paragraph separator, or a format character such as a zero-width space.
func invisible(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
}
