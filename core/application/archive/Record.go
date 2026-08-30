// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Op is what one line of a data file says happened to a row.
//
// Two values, not the change log's three. ACCESS_REVOKED is a statement about one device's view
// and means nothing in an archive - an archive is the tenant's data, not a particular actor's
// sight of it (offline-sync.md §6).
type Op string

const (
	// OpUpsert is the row as it stood at the snapshot. Creation and change are one operation for
	// the same reason they are one in the change log: the reader does not know which of the two
	// it is looking at, and does not need to.
	OpUpsert Op = "UPSERT"
	// OpDelete is a tombstone: the row was there in the parent archive and is gone now.
	//
	// Carrying these is what makes an incremental chain honest. A chain that only carried upserts
	// would restore every object that ever existed, and the ones deleted three runs ago would come
	// back looking current - the defect BK-3 and BK-6 exist to catch.
	OpDelete Op = "DELETE"
)

func (o Op) Valid() bool { return o == OpUpsert || o == OpDelete }

// Blob is a reference from a record to a file that lives under media/ rather than inside the line.
//
// The digest is the address: a medium is stored at MediaName(digest), so two records referencing
// the same bytes reference the same object and an incremental run can tell, without asking the
// object store, that a file it already transferred has not changed.
type Blob struct {
	// Digest is the lower-case hexadecimal SHA-256 of the content, as the media object recorded
	// it - never recomputed by the archive writer, which would mean reading every attachment in
	// order to decide whether to read it.
	Digest string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Record is one line of a data file.
//
// It is deliberately flat and deliberately dumb: an identity, what happened, when, and the row's
// fields as they were. No nesting of children inside parents, because a parent that carried its
// children could not be written until they were all in memory - and the restore order in Entities
// puts parents first anyway.
type Record struct {
	// ID is the row's identity within its entity. For a table with a composite primary key it is
	// the key's parts joined by "/" in the order the schema declares them, which is what lets a
	// DELETE line in a later archive name the same row an UPSERT named in an earlier one.
	ID string `json:"id"`
	Op Op     `json:"op"`
	// UpdatedAt is what an incremental run selects on. On a tombstone it is the deletion's time.
	UpdatedAt time.Time `json:"updated_at"`
	// Data is the row. Nil on a tombstone - a deletion carries no content, for the reason
	// sync.Change gives and one more: the fields of a row that is being deleted are exactly the
	// personal data an erasure was meant to remove.
	//
	// Written even when it is empty, which is deliberate: `omitempty` would make a row with no
	// fields beyond its identity - an item_label is exactly that - indistinguishable from a line
	// that lost its payload. An UPSERT always carries an object and a DELETE always carries null,
	// so a reader never has to guess which of the two an absent field meant.
	Data map[string]any `json:"data"`
	// Blobs are the media this row references, if any. Absent on a tombstone.
	Blobs []Blob `json:"blobs,omitempty"`
}

// Validate reports what makes a line unusable. It is called on write and on read: a writer that
// produced a bad line and a target that handed one back are the same problem to whoever is
// restoring, and finding it on the way out is finding it a month earlier.
func (r Record) Validate() error {
	invalid := func(reason string) error {
		return shared.ErrValidation.WithDetail(CodeRecordInvalid).
			WithParams(map[string]string{"reason": reason}).WithCause(errors.New(reason))
	}
	switch {
	case r.ID == "":
		return invalid("id")
	case !r.Op.Valid():
		return invalid("op")
	case r.UpdatedAt.IsZero():
		return invalid("updated_at")
	case r.Op == OpDelete && (len(r.Data) > 0 || len(r.Blobs) > 0):
		return invalid("tombstone_carries_content")
	case r.Op == OpUpsert && r.Data == nil:
		return invalid("data")
	}
	for _, blob := range r.Blobs {
		if !looksLikeDigest(blob.Digest) {
			return invalid("blobs.sha256")
		}
	}
	return nil
}

// looksLikeDigest checks the one thing a content address has to be, because it becomes a key at a
// target: 64 lower-case hexadecimal characters and nothing else. A digest taken on trust would be
// a path the caller chose (CodeKeyInvalid exists for the same reason on the storage port).
func looksLikeDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, c := range digest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// The refusals of a data file.
const (
	// CodeRecordInvalid is a line this build will not write or will not use.
	CodeRecordInvalid = "backup.archive_record_invalid"
	// CodeRecordUnreadable is a line that is not JSON, or one longer than any record has business
	// being.
	CodeRecordUnreadable = "backup.archive_record_unreadable"
)

// recordMaxBytes bounds one line.
//
// A comment is capped at 20000 characters and an item's description is the largest field there is;
// four mebibytes is far beyond any row and still small enough that a file arriving from somebody
// else's storage cannot exhaust the process one line at a time (T-17).
const recordMaxBytes = 4 << 20

// WriteRecords writes lines to w and answers how many it wrote.
//
// JSON Lines rather than a JSON array, and that is the property the whole format rests on: a
// writer appends one line at a time and never holds the file, and a reader takes one line at a
// time and never holds it either. An array would need its closing bracket, which means a writer
// that has to come back to the end - over a protocol where coming back means a second request.
func WriteRecords(w io.Writer, records func(yield func(Record) error) error) (int64, error) {
	buffered := bufio.NewWriter(w)
	encoder := json.NewEncoder(buffered)
	var count int64

	err := records(func(record Record) error {
		if err := record.Validate(); err != nil {
			return err
		}
		if err := encoder.Encode(record); err != nil {
			return shared.Internalf("archive: write record: %w", err)
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	if err := buffered.Flush(); err != nil {
		return count, shared.Internalf("archive: flush records: %w", err)
	}
	return count, nil
}

// ReadRecords hands every line of a data file to yield, in the order it was written.
//
// The order matters and is part of the format: within one entity a later line supersedes an
// earlier one, which is what lets a restore apply a file in one pass without sorting it first.
func ReadRecords(r io.Reader, yield func(Record) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), recordMaxBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(string(line)))
		decoder.DisallowUnknownFields()
		var record Record
		if err := decoder.Decode(&record); err != nil {
			return shared.ErrValidation.WithDetail(CodeRecordUnreadable).WithCause(err)
		}
		if err := record.Validate(); err != nil {
			return err
		}
		if err := yield(record); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return shared.ErrValidation.WithDetail(CodeRecordUnreadable).WithCause(err)
	}
	return nil
}

// Entity is one aggregate's data file: what it is called in the archive, and which table it came
// from.
//
// The two names are kept apart because they answer to different masters. The table name follows
// the schema and changes with a migration; the archive name is a wire format and may not change at
// all. Today every pair matches except in number, and that is a coincidence worth being able to
// break without a format version.
type Entity struct {
	// Name is the file under data/, without the extension.
	Name string
	// Table is where the rows come from.
	Table string
	// Optional marks an entity a run may leave out. Only the audit trail is: `include_audit` is a
	// schedule setting, because including it gives better evidence and keeps personal metadata
	// longer (backup-restore.md §7).
	Optional bool
	// Whole marks an entity that cannot say when one of its rows changed, and is therefore
	// written complete in every archive - including an incremental one - with a restore replacing
	// its set rather than merging into it.
	//
	// It is a property of the schema rather than a choice. A join table has no timestamp at all,
	// and a configuration row that carries a creation stamp and no change stamp cannot tell an
	// edit from silence. Deriving "unchanged" from a column that does not move is how an
	// incremental chain quietly loses an edit, and there is no column to add here that would not
	// be a lie until every writer maintained it (E-05).
	//
	// All of them are small by nature - the join tables and the configuration - so writing them
	// whole costs little, and it is the reading that has to be right.
	Whole bool
	// Keys are the columns that make up a record's identity, in the order Record.ID joins them
	// with "/" - the primary key without `tenant_id`, because the tenant comes from the scope a
	// restore runs in rather than from the archive (E-06).
	//
	// Declared rather than derived. A restore has to be able to say what a line identifies
	// without a database to ask, which is the whole premise of §8.1, and a build that read the
	// key from its own schema would read a schema the archive was not written under.
	Keys []string
	// References are the fields that point at another entity's rows. Only the ones inside the
	// archive: a reference to something a restore does not import is not a reference it can do
	// anything about.
	//
	// They exist for one purpose - DUPLICATE, which has to give the copy new identities and keep
	// the copies pointing at each other rather than at the originals - and they are kept honest
	// by a test that compares them against the foreign keys the database actually has.
	References []Reference
	// Duplicable says whether a collision in this entity may be settled by making a copy.
	//
	// Not everything may. An account is who somebody is, a label is the same label, a media
	// object is the same bytes under a content address, and a webhook subscription copied is a
	// subscription that fires twice. For those a DUPLICATE restore falls back to SKIP and says so
	// in the report - the alternative is a merge that quietly doubles the tenant's identities and
	// its outbound integrations.
	Duplicable bool
}

// Reference is one field pointing at another entity's rows.
type Reference struct {
	// Field is the column in this entity's data.
	Field string
	// Table is what it points at, in the schema's vocabulary - the same word the entity's own
	// Table is written in.
	Table string
}

// entities is the archive's content, in restore order: a row's parents come before it, so that a
// restore can apply the files in sequence without deferring foreign keys.
//
// The order is the format. A restore in a later version reads this list from the archive's own
// files rather than from here, but a writer that emitted them in another order would produce an
// archive that only restores if the reader sorts it - and sorting requires knowing the graph,
// which is exactly the knowledge the order exists to avoid needing.
var entities = []Entity{
	// The tenant itself, so that a NEW_TENANT restore has something to create rather than
	// something to fill in (backup-restore.md §8.2).
	{Name: "tenants", Table: "tenant", Keys: []string{"id"}},
	{Name: "accounts", Table: "account", Keys: []string{"id"}},
	{Name: "account_groups", Table: "account_group", Keys: []string{"id"}, Whole: true},
	{
		Name: "account_group_members", Table: "account_group_member", Whole: true,
		Keys: []string{"group_id", "account_id"},
		References: []Reference{
			{Field: "group_id", Table: "account_group"},
			{Field: "account_id", Table: "account"},
		},
	},
	{
		Name: "memberships", Table: "membership", Whole: true, Keys: []string{"id"},
		References: []Reference{
			{Field: "account_id", Table: "account"},
			{Field: "group_id", Table: "account_group"},
		},
	},
	{
		Name: "containers", Table: "container", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "parent_id", Table: "container"}},
	},
	{
		Name: "buckets", Table: "bucket", Whole: true, Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "collection_id", Table: "container"}},
	},
	{
		Name: "labels", Table: "label", Whole: true, Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "collection_id", Table: "container"}},
	},
	{
		Name: "custom_field_definitions", Table: "custom_field_definition",
		Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "collection_id", Table: "container"}},
	},
	{
		Name: "work_items", Table: "work_item", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{
			{Field: "collection_id", Table: "container"},
			{Field: "parent_id", Table: "work_item"},
			{Field: "bucket_id", Table: "bucket"},
			{Field: "assignee_id", Table: "account"},
			{Field: "cover_media_id", Table: "media_object"},
			// No foreign key in the schema, and a reference all the same: the column holds a
			// recurrence rule's identity, and a duplicate that kept the original's would make two
			// items claim one series.
			{Field: "recurrence_rule_id", Table: "recurrence_rule"},
			{Field: "origin_jumble_id", Table: "jumble_entry"},
		},
	},
	{
		Name: "item_labels", Table: "item_label", Whole: true, Duplicable: true,
		Keys: []string{"item_id", "label_id"},
		References: []Reference{
			{Field: "item_id", Table: "work_item"},
			{Field: "label_id", Table: "label"},
		},
	},
	{
		Name: "item_members", Table: "item_member", Whole: true, Duplicable: true,
		Keys: []string{"item_id", "account_id"},
		References: []Reference{
			{Field: "item_id", Table: "work_item"},
			{Field: "account_id", Table: "account"},
		},
	},
	{
		Name: "comments", Table: "comment", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{
			{Field: "item_id", Table: "work_item"},
			{Field: "parent_comment_id", Table: "comment"},
		},
	},
	{
		Name: "activity_entries", Table: "activity_entry", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "item_id", Table: "work_item"}},
	},
	// Not duplicable: a medium is its bytes, addressed by their checksum, and a second row for
	// the same bytes is a second reference count to keep honest for no gain.
	{Name: "media_objects", Table: "media_object", Whole: true, Keys: []string{"id"}},
	{
		Name: "item_attachments", Table: "item_attachment", Whole: true, Duplicable: true,
		Keys: []string{"item_id", "media_id"},
		References: []Reference{
			{Field: "item_id", Table: "work_item"},
			{Field: "media_id", Table: "media_object"},
		},
	},
	{
		Name: "recurrence_rules", Table: "recurrence_rule", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "source_item_id", Table: "work_item"}},
	},
	{
		Name: "reminders", Table: "reminder", Keys: []string{"id"}, Duplicable: true,
		References: []Reference{{Field: "item_id", Table: "work_item"}},
	},
	{Name: "saved_views", Table: "saved_view", Whole: true, Keys: []string{"id"}},
	{Name: "templates", Table: "template", Keys: []string{"id"}},
	{Name: "jumble_entries", Table: "jumble_entry", Keys: []string{"id"}},
	{Name: "auto_assign_policies", Table: "auto_assign_policy", Whole: true, Keys: []string{"id"}},
	{
		Name: "automation_rules", Table: "automation_rule", Keys: []string{"id"},
		References: []Reference{{Field: "run_as", Table: "account"}},
	},
	// Not duplicable, and this is the one where a copy would be actively harmful: two
	// subscriptions on one event send everything twice, to somebody who never asked for it.
	{Name: "webhook_subscriptions", Table: "webhook_subscription", Whole: true, Keys: []string{"id"}},
	{
		Name: "calendar_feeds", Table: "calendar_feed", Whole: true, Keys: []string{"id"},
		References: []Reference{
			{Field: "account_id", Table: "account"},
			{Field: "view_id", Table: "saved_view"},
		},
	},
	{
		Name: "notification_preferences", Table: "notification_preference",
		Keys:       []string{"account_id", "category", "channel"},
		References: []Reference{{Field: "account_id", Table: "account"}},
	},
	{Name: "retention_policies", Table: "retention_policy", Keys: []string{"data_kind"}},
	{
		Name: "consent_records", Table: "consent_record", Whole: true, Keys: []string{"id"},
		References: []Reference{{Field: "account_id", Table: "account"}},
	},
	{Name: "legal_holds", Table: "legal_hold", Whole: true, Keys: []string{"id"}},
	{
		Name: "set_elements", Table: "set_element", Whole: true, Duplicable: true,
		Keys:       []string{"item_id", "set_name", "element_id"},
		References: []Reference{{Field: "item_id", Table: "work_item"}},
	},
	// Last, and optional. It is the only file whose absence is a configuration rather than a
	// defect - and the only one a restore reads and deliberately does not write back, because the
	// live trail is a hash chain and an insert into the middle of one is not a restore but a
	// rewrite (E-06, audit.md §4).
	{Name: "audit", Table: "audit_log", Optional: true, Keys: []string{"seq"}},
}

// Entities is what an archive holds, in restore order.
func Entities() []Entity { return slices.Clone(entities) }

// notRestored is every entity an archive carries and a restore deliberately does not write back,
// and why (E-06).
//
// A map rather than a comment, for the reason `excluded` is one: the reasons are read by the test
// beside it and by anybody wondering where their data went. The difference between this list and
// that one is what happened at the other end - `excluded` is not in the archive at all, and this
// is in the archive and stays there.
var notRestored = map[string]string{
	// audit.md §4 makes the trail a hash chain: each entry carries the digest of the one before
	// it, and E-09's `:verify` walks that chain. Inserting last month's entries into the middle of
	// a live chain is not a restore, it is a rewrite - and "the trail cannot be rewritten" is the
	// property the whole audit surface rests on. The archive still carries it, which is what
	// `include_audit` was for: the evidence is readable where it was written down.
	"audit_log": "audit.md §4 - the live trail is a hash chain, and an insert into one is a rewrite",
}

// NotRestored answers what a restore reads and does not write back, and why.
func NotRestored() map[string]string { return maps.Clone(notRestored) }

// RestoredEntities are the entities a restore writes, in the order it writes them: a row's parents
// before the row, so that the files are applied in sequence without deferring a foreign key.
func RestoredEntities() []Entity {
	out := make([]Entity, 0, len(entities))
	for _, entity := range entities {
		if _, kept := notRestored[entity.Table]; !kept {
			out = append(out, entity)
		}
	}
	return out
}

// HasOwnIdentity reports an entity whose identity is a single `id` column of its own rather than a
// key made of references to other entities.
//
// It is the question DUPLICATE asks: an entity with an identity of its own needs a new one minted
// for the copy, and a join table's identity follows from the rows it joins - remap those and the
// key has changed by itself.
func (e Entity) HasOwnIdentity() bool {
	return len(e.Keys) == 1 && e.Keys[0] == "id"
}

// FindEntityByTable answers the entity whose rows come from that table.
//
// It exists because the deletion markers are written in the schema's vocabulary - the lifecycle
// records a tombstone against `work_item`, not against `work_items` - and an exporter turning
// those into archive lines has to cross from one naming to the other exactly once.
func FindEntityByTable(table string) (Entity, bool) {
	index := slices.IndexFunc(entities, func(e Entity) bool { return e.Table == table })
	if index < 0 {
		return Entity{}, false
	}
	return entities[index], true
}

// WholeEntities are the entities written complete in every archive, by name.
func WholeEntities() []string {
	var names []string
	for _, entity := range entities {
		if entity.Whole {
			names = append(names, entity.Name)
		}
	}
	return names
}

// FindEntity answers the entity of that name.
func FindEntity(name string) (Entity, bool) {
	index := slices.IndexFunc(entities, func(e Entity) bool { return e.Name == name })
	if index < 0 {
		return Entity{}, false
	}
	return entities[index], true
}

// excluded is every tenant-scoped table an archive deliberately leaves out, and why.
//
// It is a map rather than a comment because of the test beside it: every table the schema puts
// under row level security is in exactly one of the two lists, so a table added by a later
// migration turns the gate red until somebody decides which it is. "Nobody thought about it" is
// how a restore quietly loses a feature's data.
var excluded = map[string]string{ //nolint:gosec // G101: table names and prose, one of which is called access_token
	// §8.4, in as many words: no tokens or sessions are restored. Making credentials from an
	// archive valid again is a security risk, and the archive is the easier of the two places to
	// steal them from.
	"access_token":  "§8.4 - a restore does not make old credentials valid again",
	"sync_device":   "a device registration carries a push token, and §8.4's reasoning covers it",
	"jumble_intake": "the intake token's hash is a credential store, and §8.4's reasoning covers it: a restored address would open the inbox to whoever held the old token",
	// §8.4, the automation half. A restored run log would describe runs of a period that is being
	// replayed without firing anything, which is a record of things that did not happen.
	"rule_run": "§8.4 - no automation fires during a restore, so its run log would be fiction",
	// The moments the relative-date rules owe (G-08). Derived from the entries and the rules, both
	// of which the archive carries, and every one of them is a moment in the *source* system's
	// future - a restore months later would owe a night that has long passed. The rules themselves
	// are restored; what they owe is worked out again from the anchors, as it always is.
	"rule_occurrence":   "§8.4 - a debt owed at a moment in the source system's future; the rules are restored, what they owe is recomputed",
	"webhook_delivery":  "§8.4 - nothing is re-delivered",
	"outbox_event":      "§8.4 - the archive's outbox is not imported",
	"event_consumption": "the outbox's companion; without the outbox it says nothing",
	"notification":      "a notification announces a change; restoring one announces last month's",
	// Guards and counters that are about requests rather than about data.
	"idempotency_key": "a replay guard for requests that were over before the archive was written",
	"sync_op_log":     "the same, for the sync push - a 30-day window, not data",
	"usage_record":    "the operator's billing ledger rather than the tenant's data",
	// The cursor and the markers of the live system. Restoring them would hand devices numbers
	// that no longer mean anything.
	"change_log": "the offline cursor; its sequence numbers belong to the database that issued them",
	"tombstone":  "carried as DELETE lines inside each entity's file, which is what makes a chain complete",
	// The compliance machinery. Each of these is a live case with a deadline or an attestation,
	// and a restored copy would revive a clock that has already run out.
	"audit_anchor":         "it attests to the chain in the live audit log, not to a copy of one (E-09)",
	"audit_pseudonym":      "the result of an erasure over a trail that is never written back; carrying it would let a restore reinstate a name, or remove one it did not (E-10)",
	"data_subject_request": "a case with a legal deadline; a restored one revives a deadline that has passed (E-10)",
	"privacy_incident":     "the same reasoning: an incident is handled once",
	// The backup system's own bookkeeping. An archive describing the runs that produced it would
	// be a mirror facing a mirror.
	"backup_target":   "an egress channel and a sealed credential; a restore must not silently recreate one",
	"backup_schedule": "the backup system's own bookkeeping",
	"backup_run":      "the backup system's own bookkeeping",
	"restore_run":     "the backup system's own bookkeeping",
	"retention_run":   "the retention engine's own bookkeeping (E-07)",
	// A rule that says EXPORT_THEN_DELETE names a backup target, and a backup target is
	// deliberately not restored - "an egress channel and a sealed credential; a restore must not
	// silently recreate one". A rule carried back without it would either refuse the insert or,
	// worse, become a plain deletion whose export half is gone. `retention_policy` is archived
	// because it is only a period: it removes nothing on its own (E-07).
	"retention_rule":   "a rule that can delete, pointing at an egress channel a restore does not recreate",
	"deletion_journal": "§7 - the journal is applied to a restore rather than restored by one",
	// And the one row-level-secured table that is nobody's data: the system-defined capability
	// profiles every tenant may read and none may write. They come with the installation, and a
	// restore that carried its own copy would be a restore that could contradict it.
	"item_capability_profile": "installation-wide and system-defined; it arrives with the build, not with the data",
}

// ExcludedTables answers what an archive leaves out and why. The reasons are read by the test that
// keeps the two lists total, and by anybody wondering where their table went.
func ExcludedTables() map[string]string { return maps.Clone(excluded) }

// String makes an entity printable in a test failure without spelling out both names by hand.
func (e Entity) String() string { return fmt.Sprintf("%s (%s)", e.Name, e.Table) }
