// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import "slices"

// Anchor is the column a period runs from (data-retention.md §3).
//
// A property of the data kind rather than of the rule, because "a year after what" is not something
// a tenant gets to choose: a completed item's year runs from the completion, and a rule that could
// point the period at another column would be a rule that could keep work for a year after it was
// created and delete it while it was still open.
type Anchor string

const (
	AnchorCompletedAt Anchor = "completed_at"
	AnchorUpdatedAt   Anchor = "updated_at"
	AnchorDeletedAt   Anchor = "deleted_at"
	AnchorArchivedAt  Anchor = "archived_at"
	AnchorCreatedAt   Anchor = "created_at"
	AnchorOccurredAt  Anchor = "occurred_at"
	AnchorStartedAt   Anchor = "started_at"
	AnchorLastSeenAt  Anchor = "last_seen_at"
)

// Action is what a rule does when the period is up (data-retention.md §2).
type Action string

const (
	// ActionArchive puts the object out of the way and keeps it. The mildest of the six, and the
	// one §2 recommends in front of a deletion: "completed work has a habit of turning out to be
	// relevant after all".
	ActionArchive Action = "ARCHIVE"
	// ActionTrash puts the object in the trash, where the trash's own period then applies to it.
	ActionTrash Action = "TRASH"
	// ActionAnonymize strips what identifies a person and keeps the rest. It belongs to the
	// erasure machinery of E-10 rather than to this engine, and until that exists a rule asking
	// for it is refused rather than treated as a deletion.
	ActionAnonymize Action = "ANONYMIZE"
	// ActionHardDelete removes the object for good, with the journal entry and the tombstone that
	// make the removal survive a restore and a stale device.
	ActionHardDelete Action = "HARD_DELETE"
	// ActionExportThenDelete writes an archive to the rule's backup target and then removes the
	// object (§6).
	ActionExportThenDelete Action = "EXPORT_THEN_DELETE"
	// ActionNotifyOnly announces and does nothing. It is what a broadly matching rule starts in
	// (§5) and what §3 gives OPEN_ITEM_STALE as its only sensible default - automatically deleting
	// open work is dangerous.
	ActionNotifyOnly Action = "NOTIFY_ONLY"
)

var actions = [...]Action{
	ActionArchive, ActionTrash, ActionAnonymize,
	ActionHardDelete, ActionExportThenDelete, ActionNotifyOnly,
}

// Valid reports whether an action is one the model defines. Whether this build can *perform* it on
// a given kind is a different question - see Kind.Performs.
func (a Action) Valid() bool { return slices.Contains(actions[:], a) }

// Removes reports the actions after which there is no object left to act on again, which is what
// makes them the end of a chain.
func (a Action) Removes() bool {
	return a == ActionHardDelete || a == ActionExportThenDelete || a == ActionAnonymize
}

// Leaves is the column an action writes, and therefore the anchor a second stage counts from.
//
// This is the whole of what "the anchor of each stage taken from the right column" means: a chain
// of completed → archive after a year → delete after two more does not delete two years after the
// completion, it deletes two years after the archiving. Reading the original anchor for the second
// stage would collapse a three-year chain into a two-year one, silently.
//
// An action that leaves no column has no second stage, which is what Removes says the other way
// round.
func (a Action) Leaves() (Anchor, bool) {
	switch a {
	case ActionArchive:
		return AnchorArchivedAt, true
	case ActionTrash:
		return AnchorDeletedAt, true
	}
	return "", false
}

// Kind is one class of data and everything the engine needs to know about it (data-retention.md §3).
//
// The catalogue is data rather than code in the sense ADR-0020 means: a rule is configured against
// an entry here and the engine reads the entry, so a new kind is one row plus whatever sweeps it -
// never a branch inside the engine. What an entry cannot invent is a sweeper, which is why Swept is
// a field: a kind nothing removes would be a promise nothing keeps, and refusing to configure one
// is more honest than accepting a rule that never runs.
type Kind struct {
	Name DataKind
	// Anchor is the column the period runs from.
	Anchor Anchor
	// DefaultDays is the period a tenant starts with, and zero means the kind is off by default -
	// which §3 gives for most of them, because deleting somebody's work by default is not a
	// default anybody chose.
	DefaultDays int
	// MinDays is what no rule may undercut (§4.3).
	MinDays int
	// MaxDays is the upper bound where the operator has set one, and nil where there is none.
	// Exceeding it needs a justification and writes an audit entry (§4.4).
	MaxDays *int
	// Actions are what this build can do to this kind. An action outside the set is refused at
	// configuration time rather than skipped at run time.
	Actions []Action
	// Marks says whether the kind goes through the marking phase (§5).
	//
	// False for the trash, and that is the decision `db/queries/Lifecycle.sql` already records: the
	// trash is its own grace period - the object is visible, it can be taken out, and it has a date
	// - so a MARK pass would be a second grace period on top of a grace period. False for the
	// notification history too, for a plainer reason: nobody can take a notification record out,
	// so announcing its removal would be an announcement with no action attached.
	Marks bool
	// Blockable are the reasons an object of this kind can be kept back, which is what the metric's
	// label set is drawn from. A kind nothing can block reports no series rather than a zero that
	// can never be anything else.
	Blockable []string
}

// Swept reports whether this build has something that removes this kind.
func (k Kind) Swept() bool { return len(k.Actions) > 0 }

// Performs reports whether this build can do that to this kind.
func (k Kind) Performs(action Action) bool { return slices.Contains(k.Actions, action) }

// Ceiling is the kind's upper bound, and zero when it has none.
func (k Kind) Ceiling() int {
	if k.MaxDays == nil {
		return 0
	}
	return *k.MaxDays
}

// catalogue is data-retention.md §3, in the same order.
//
// Every kind the document names is here, including the ones nothing sweeps yet. That is deliberate:
// the list is what `/meta/capabilities` answers and what a client offers, and a kind that were
// simply absent would look like a kind the document invented. What separates them is Actions -
// empty means "named here, nothing removes it", and a rule for one of those is refused with a code
// that says which of the two it is.
var catalogue = []Kind{
	{
		Name: KindCompletedItem, Anchor: AnchorCompletedAt, MinDays: 0, Marks: true,
		Actions: []Action{
			ActionArchive, ActionTrash, ActionHardDelete, ActionExportThenDelete, ActionNotifyOnly,
		},
		Blockable: []string{
			BlockedByLegalHold, BlockedByRestriction, BlockedByTombstoneWindow, BlockedByDescendant,
		},
	},
	// Named, and swept by nothing. §3 gives it NOTIFY_ONLY as its only sensible default -
	// automatically deleting open work is dangerous - and an announcement with no engine behind it
	// is not something to offer before the announcement exists.
	{Name: KindOpenItemStale, Anchor: AnchorUpdatedAt},
	{
		Name: KindTrash, Anchor: AnchorDeletedAt, DefaultDays: 30, MinDays: 7,
		Actions:   []Action{ActionHardDelete},
		Blockable: []string{BlockedByLegalHold, BlockedByRestriction, BlockedByTombstoneWindow},
	},
	{
		Name: KindArchivedItem, Anchor: AnchorArchivedAt, Marks: true,
		Actions: []Action{ActionTrash, ActionHardDelete, ActionExportThenDelete, ActionNotifyOnly},
		Blockable: []string{
			BlockedByLegalHold, BlockedByRestriction, BlockedByTombstoneWindow, BlockedByDescendant,
		},
	},
	{Name: KindComment, Anchor: AnchorCreatedAt},
	{Name: KindAttachment, Anchor: AnchorCreatedAt},
	{
		// The jumble (G-10). Ninety days from the arrival, and the closed-set change D-06
		// predicted: the kind was named here with nothing behind it until the inbox existed.
		//
		// No marking phase, for the trash's reason turned around. An entry has no grace period to
		// announce into - nobody can take one back out of a period the way `:retain` takes an item
		// out - and what governs it is its period and the decision made about it. What is due is
		// what was never converted; an entry that became a work item is that item's provenance and
		// stays.
		Name: KindJumbleEntry, Anchor: AnchorCreatedAt, DefaultDays: 90,
		Actions: []Action{ActionHardDelete},
		// A tenant-wide hold, and nothing narrower. An entry sits in no container and under no
		// item - it is what arrived before anybody decided where it belongs - so a hold on a hub
		// has nothing to say about it, and "freeze this tenant" has everything to say about it.
		Blockable: []string{BlockedByLegalHold},
	},
	{
		Name: KindNotification, Anchor: AnchorCreatedAt, DefaultDays: 90,
		Actions: []Action{ActionHardDelete},
	},
	{Name: KindActivityEntry, Anchor: AnchorOccurredAt},
	{Name: KindRuleRun, Anchor: AnchorStartedAt, DefaultDays: 30},
	{Name: KindWebhookDelivery, Anchor: AnchorCreatedAt, DefaultDays: 30},
	{
		// The outbox's own rows (G-02). ADR-0007's second countermeasure and, until it existed,
		// the one table in this schema that only ever grew: an event's job is done the moment
		// every consumer has had it, and the row lives on afterwards for the day somebody has to
		// reconstruct what was published.
		//
		// Seven days, the shortest default in the catalogue. It is a debugging aid rather than a
		// record - the audit trail is the record - and a week covers "what happened on Friday"
		// asked on Monday.
		Name: KindOutboxEvent, Anchor: AnchorOccurredAt, DefaultDays: 7,
		Actions: []Action{ActionHardDelete},
	},
	{Name: KindSession, Anchor: AnchorLastSeenAt, DefaultDays: 30},
	{Name: KindAudit, Anchor: AnchorOccurredAt, DefaultDays: 400},
	{Name: KindMediaOrphan, Anchor: AnchorCreatedAt, DefaultDays: 7},
	{Name: KindDeletedAccountResidue, Anchor: AnchorDeletedAt, DefaultDays: 30},
}

// The classes of data data-retention.md §3 names. Every one of them, including the ones nothing
// removes yet: the catalogue is the document's, and a subset would be this build's opinion of it.
const (
	KindCompletedItem   DataKind = "COMPLETED_ITEM"
	KindOpenItemStale   DataKind = "OPEN_ITEM_STALE"
	KindArchivedItem    DataKind = "ARCHIVED_ITEM"
	KindComment         DataKind = "COMMENT"
	KindAttachment      DataKind = "ATTACHMENT"
	KindJumbleEntry     DataKind = "JUMBLE_ENTRY"
	KindActivityEntry   DataKind = "ACTIVITY_ENTRY"
	KindRuleRun         DataKind = "RULE_RUN"
	KindWebhookDelivery DataKind = "WEBHOOK_DELIVERY"
	// KindOutboxEvent is how long a dispatched event stays readable after every consumer has
	// had it (G-02, ADR-0007's second countermeasure).
	KindOutboxEvent           DataKind = "OUTBOX_EVENT"
	KindSession               DataKind = "SESSION"
	KindAudit                 DataKind = "AUDIT"
	KindMediaOrphan           DataKind = "MEDIA_ORPHAN"
	KindDeletedAccountResidue DataKind = "DELETED_ACCOUNT_RESIDUE"
)

// Catalogue is every kind the document names.
func Catalogue() []Kind { return slices.Clone(catalogue) }

// FindKind answers one entry of the catalogue.
func FindKind(name DataKind) (Kind, bool) {
	index := slices.IndexFunc(catalogue, func(k Kind) bool { return k.Name == name })
	if index < 0 {
		return Kind{}, false
	}
	return catalogue[index], true
}

// SweptKinds are the kinds this build actually removes, which is what the engine iterates.
func SweptKinds() []Kind {
	var swept []Kind
	for _, kind := range catalogue {
		if kind.Swept() {
			swept = append(swept, kind)
		}
	}
	return swept
}
