// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "slices"

// DataClass is what a piece of data *is*, in the six classes `data-protection.md` §3 names and
// ADR-0018 decision 1 makes a property of the model rather than a document (E-11).
//
// One vocabulary, here, because there were three. The concept named six classes, the data
// catalogue's legend named five - it had lost `SPECIAL_CATEGORY_RISK` - and `audit.md` §4 named
// three, which are not classes at all but a *masking* policy derived from them. A gate cannot
// reconcile three vocabularies; it can reconcile one and a derivation, which is what this type and
// `audit.MaskingFor` are.
type DataClass string

const (
	// ClassNonPersonal is configuration, enum values, counters. No restriction.
	ClassNonPersonal DataClass = "NON_PERSONAL"
	// ClassPersonalBasic is a name, an address, an avatar, a locale: a person's identity rather
	// than their work.
	ClassPersonalBasic DataClass = "PERSONAL_BASIC"
	// ClassPersonalContent is what somebody wrote: titles, notes, comments, attachments, the
	// history of an entry.
	ClassPersonalContent DataClass = "PERSONAL_CONTENT"
	// ClassPersonalTechnical is an address, a user agent, a device characteristic - stored
	// truncated and kept briefly.
	ClassPersonalTechnical DataClass = "PERSONAL_TECHNICAL"
	// ClassSpecialCategoryRisk is the class the product does not collect and cannot prevent: a
	// task manager holds no health data, and "reschedule MRI appointment, oncology" sits in a free
	// text field (Art. 9). Naming it is what makes the care around free text deliberate.
	ClassSpecialCategoryRisk DataClass = "SPECIAL_CATEGORY_RISK"
	// ClassSecret is a password, a token, a key. Only hashed or encrypted, never exported, never
	// audited.
	ClassSecret DataClass = "SECRET"
)

var dataClasses = [...]DataClass{
	ClassNonPersonal, ClassPersonalBasic, ClassPersonalContent,
	ClassPersonalTechnical, ClassSpecialCategoryRisk, ClassSecret,
}

// DataClasses returns every class, in the order §3's table lists them.
func DataClasses() []DataClass { return dataClasses[:] }

// Valid reports whether the class is one of the six.
func (c DataClass) Valid() bool { return slices.Contains(dataClasses[:], c) }

// Personal reports whether the class describes a person. It is the question every gate over the
// data catalogue asks: `NON_PERSONAL` needs no deletion path, and everything else does.
func (c DataClass) Personal() bool {
	return c.Valid() && c != ClassNonPersonal
}
