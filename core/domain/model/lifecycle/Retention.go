// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import "time"

// DataKind is a class of data the lifecycle context keeps a period for (data-retention.md §3).
//
// The catalogue is data rather than code by design: a new kind is added to the document and is then
// configurable through the API without an engine change. Catalogue.go is that catalogue - every
// kind the document names, with the ones this build can actually remove marked as such, because a
// period configured for a kind nothing sweeps would be a promise nothing keeps.
type DataKind string

// KindTrash is the period a deletion waits out before it becomes permanent (F-09).
const KindTrash DataKind = "TRASH"

// KindNotification is how long the record of somebody having been told is kept (C-09).
const KindNotification DataKind = "NOTIFICATION"

// Policy is one tenant's period for one kind of data, as `retention_policy` holds it.
//
// Superseded by Rule (E-07) and kept for the length of one release: the old table's key allows one
// period per kind per tenant, and the rule model is scoped. Its rows are carried into
// `retention_rule` by the first sweep after the upgrade, and a later release contracts it away -
// which is what expand-before-contract means for a table an old pod is still writing to.
type Policy struct {
	DataKind DataKind
	// RetainDays is the period from the kind's time anchor, which for the trash is when the row went
	// in (data-retention.md §3).
	RetainDays int
	// MinDays is the documented lower bound: what stops a misconfigured rule deleting a trash the
	// moment something lands in it. Seven days for the trash.
	MinDays int
	// MaxDays is the upper bound where the operator has set one, and nil where there is none.
	// Exceeding it needs a justification, which is checked where a rule is written rather than here.
	MaxDays *int
}

// DefaultPolicies are the periods a tenant starts with, as data-retention.md §3 documents them.
//
// Here rather than in a migration, and read by the run rather than assumed: a tenant that has no row
// yet gets these written for it on the first sweep, so a tenant created after the migration is not a
// tenant with no policy. The defaults are the privacy-friendly ones of Art. 25(2)
// (data-protection.md §5).
func DefaultPolicies() []Policy {
	return []Policy{
		{DataKind: KindTrash, RetainDays: 30, MinDays: 7},
		// Ninety days, as data-retention.md §3 and data-protection.md §5 both give it.
		//
		// No lower bound, and that is a decision rather than an omission. §4.3 makes lower bounds a
		// precedence rule and names exactly one - "trash, for example, is at least 7 days" - and
		// nothing in the documents bounds the notification history. Inventing a floor here would be
		// a retention rule decided in code, which is the one place ADR-0020 says it must not be: a
		// tenant that wants a shorter notification history is asking for less data to be kept, and
		// refusing them would be arguing against Art. 25(2) on their behalf.
		{DataKind: KindNotification, RetainDays: 90},
	}
}

// Cutoff is the instant a row has to have been deleted before, for this policy to allow its removal.
//
// The lower bound is applied here rather than trusted from the row, and it is the bound of the *kind*
// rather than the one the row carries. data-retention.md §4.3 makes it a precedence rule - "lower
// bounds per data kind prevent accidental immediate deletion; trash, for example, is at least 7
// days" - which makes it a property of the kind that a row is a copy of. A rule that read the copy
// would be undercut by whoever wrote the copy, which is the one thing a lower bound exists to
// prevent.
//
// A period below the bound is treated as the bound rather than refused: the tenant asked for their
// trash to be emptied sooner, and answering with the soonest allowed is closer to that than refusing
// to sweep at all.
func (p Policy) Cutoff(now time.Time) time.Time {
	days := p.RetainDays
	if floor := FloorFor(p.DataKind); days < floor {
		days = floor
	}
	return now.AddDate(0, 0, -days)
}

// FloorFor is the lower bound of one kind: what no configuration may undercut.
//
// Read off the documented defaults, so that the bound and the default it belongs to are one
// statement. A kind that has no default has no bound either - which is the honest answer for a kind
// nothing sweeps yet, rather than a floor invented here.
func FloorFor(kind DataKind) int {
	for _, policy := range DefaultPolicies() {
		if policy.DataKind == kind {
			return policy.MinDays
		}
	}
	return 0
}
