// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backup is the outbound port for the targets a tenant has configured (E-03).
package backup

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Targets stores and reads what a tenant has configured, and keeps the credential apart from
// everything else.
//
// Credentials is a method of its own rather than a field on a target, and that is the shape the
// whole "never returned by any read" requirement rests on: the rows that go to a response cannot
// contain a credential, because the statements behind them do not select one. A field that a
// mapper could accidentally copy is a field that eventually is.
type Targets interface {
	// Insert writes the target and its sealed credential. The credential arrives already sealed:
	// this port knows there is one and never what it says.
	Insert(ctx context.Context, target domain.Target, credential crypto.Sealed) error

	// List answers the tenant's targets, by name. Never a credential.
	List(ctx context.Context) ([]domain.Target, error)

	// Find answers one target, or ErrNotFound. Never a credential.
	Find(ctx context.Context, id shared.ID) (domain.Target, error)

	// Credential answers the sealed credential of a target, still sealed. The only caller is the
	// use case that opens a connection, and the only thing it does with the answer is hand it to
	// the encryptor.
	Credential(ctx context.Context, id shared.ID) (crypto.Sealed, error)

	// RecordTest writes down what the connection probe found: when, whether it worked, and the
	// message code when it did not. Never a driver message - one of those carries the host, the
	// user and sometimes the password (rule 10).
	RecordTest(ctx context.Context, id shared.ID, at time.Time, ok bool, code string) error

	// Coverage is what the installation's health surface asks: how many targets there are, and
	// how many of them store an archive unencrypted (backup-restore.md §10).
	Coverage(ctx context.Context) (Coverage, error)
}

// Coverage is the answer to "is this tenant backed up, and how badly".
type Coverage struct {
	Configured  int
	Unencrypted int
}

// Export is the tenant's rows as the archive writer needs them (E-05, backup-restore.md §3).
//
// It is keyed by table name rather than by the archive's entity names, and that is the seam: the
// database's vocabulary stops here, and the archive's begins on the other side. The deletion
// markers are written in the same vocabulary, so a tombstone against `work_item` and a page of
// `work_item` rows are asked for with one word.
//
// Everything is a callback rather than a slice, for the reason the archive's own Source is: the
// answer is as large as the tenant, and a method returning []Row would read a holding into memory
// before writing a byte of it (T-17). What the implementation does behind that is page on each
// entity's own key - never OFFSET - so that a page can neither repeat nor skip a row while the
// snapshot is open, and a resumed run continues where it stopped instead of counting again.
type Export interface {
	// Rows hands over one table's rows, oldest change first.
	//
	// since is exclusive and is the zero time for a whole read. A table that cannot date a change
	// is asked for whole and answers everything whatever is passed - the caller decides which
	// those are, because it is the archive that has to record the decision for a restore.
	Rows(ctx context.Context, table string, since time.Time, yield func(Row) error) error

	// Tombstones hands over one table's deletion markers after an instant, oldest first. Nothing
	// at all for a full archive, which has no earlier state to contradict.
	Tombstones(ctx context.Context, table string, since time.Time, yield func(Tombstone) error) error

	// MediaLocation answers where the bytes of one medium lie, by the checksum the archive
	// addresses it with, or ErrNotFound.
	MediaLocation(ctx context.Context, checksum string) (MediaLocation, error)
}

// Row is one row on its way into an archive.
type Row struct {
	// ID is the row's identity: the primary key, or its parts joined by "/" in the order the
	// schema declares them. It is also the cursor - the parts are split back out to ask for the
	// next page - which is why nothing that can contain a slash is ever part of one.
	ID string
	// ChangedAt is when the row last changed, and the zero time from a table that cannot say.
	ChangedAt time.Time
	// Data is the row, with `tenant_id` already removed: a restore into another tenant must not
	// carry the old one's identifier back in with it.
	Data map[string]any
}

// Tombstone is one deletion marker.
type Tombstone struct {
	ID        string
	DeletedAt time.Time
}

// MediaLocation is where one medium's bytes are, in the object store's terms.
type MediaLocation struct {
	StorageKey string
	Bytes      int64
}

// Restores stores what a restore did, and what it is about to do (E-06).
//
// The row is written when the restore is accepted rather than when it starts, which is what lets a
// caller poll the `result_url` they were handed instead of meeting a 404 for the first few seconds.
type Restores interface {
	// Insert writes the accepted restore, PENDING.
	Insert(ctx context.Context, restore domain.Restore) error

	// Find answers one restore, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Restore, error)

	// Claim moves the run to RUNNING and answers whether it got the tenant.
	//
	// False is not an error: a second restore in one tenant is what the lock exists to prevent.
	// A run that is already RUNNING claims itself again, which is what makes a job that died
	// resumable - it continues its own run rather than being told the tenant is busy (BK-7).
	Claim(ctx context.Context, id shared.ID, at time.Time) (bool, error)

	// Finish records how a restore ended and what it did. Refused for a run that is no longer
	// going, which is what makes a cancelled restore stay cancelled.
	Finish(ctx context.Context, outcome domain.RestoreOutcome) error

	// RecordProgress writes how far the run has got, and the report so far, in the transaction of
	// the batch that got it there. A resumed attempt reads both back: the progress so that it
	// does not decide the same records twice, and the report so that it continues counting rather
	// than starting again (BK-7).
	RecordProgress(ctx context.Context, id shared.ID, report domain.Report, progress map[string]int) error

	// RecordSafetyCopy writes down the backup taken before a destructive mode, before the mode
	// runs. The way back has to be findable from the run even if the run then fails.
	RecordSafetyCopy(ctx context.Context, id, backupRunID shared.ID) error

	// InProgress reports whether this tenant already has a restore going, so that a second one is
	// refused where the caller can read the refusal rather than minutes later inside a job.
	InProgress(ctx context.Context) (bool, error)
}

// Workspace is the one thing a destructive restore has to ask about the tenant it is about to
// replace: what it is called (E-06, backup-restore.md §8.3 step 3).
//
// A port of one method rather than a field on something larger, because that is genuinely all of
// it. The name is read to be compared against what somebody typed, and it is never answered to a
// caller - a use case that returned it would be a use case that told you what to type.
type Workspace interface {
	// Name answers the display name of the tenant the transaction is bound to.
	Name(ctx context.Context) (string, error)
}

// Import is the tenant's rows as a restore writes them (E-06, backup-restore.md §8).
//
// The mirror of Export, in the same vocabulary and with the same seam: table names on this side,
// the archive's entity names on the other. Row by row rather than in pages, which is the one place
// this port is deliberately less efficient than its opposite - a restore has to decide each row
// against the conflict rule and against the deletion journal, and a batch that wrote thirty rows
// at once could not report which of them it had skipped.
//
// The tenant is never a parameter. It comes from `current_tenant_id()` inside every statement, so
// a restore cannot write into another tenant even deliberately - BK-10 at the layer where it
// cannot be forgotten.
type Import interface {
	// Holds reports whether the tenant already has the row this data identifies.
	//
	// Asked before the write rather than derived from it, because the dry run has to answer the
	// same question without writing anything (§8.3 step 2). One question in both paths is also
	// what makes the report a caller approved and the report they get back comparable.
	Holds(ctx context.Context, table string, data map[string]any) (bool, error)

	// Write inserts the row, replaces it when overwrite is true, and answers whether anything was
	// written. False is a collision the caller asked to leave alone - not an error.
	Write(ctx context.Context, table string, data map[string]any, overwrite bool) (bool, error)

	// Clear empties one table within the tenant and answers how many rows went. It is what
	// REPLACE_TENANT is made of, and it exists for no other mode.
	Clear(ctx context.Context, table string) (int, error)
}

// Journal is the deletion journal, read (E-06, backup-restore.md §7).
//
// The table has been written since B-10 and read, until now, only by tests - the comment on the
// writing side says so in as many words: "nothing reads this table in production; it exists so
// that a restore from backup cannot bring back what was deleted." This is that reader.
//
// It is a port of its own rather than a method on the lifecycle repository for the reason that
// port already gives about mixing reads with deletions: one interface carrying both would let a
// read path reach a statement written to remove rows. Here the asymmetry is sharper still - the
// writer is the machinery that deletes, and the reader is the machinery that must not undelete.
type Journal interface {
	// DeletedSince hands over the deletions recorded after an instant, oldest first.
	//
	// The instant is the archive's snapshot, and that is what makes the read bounded rather than
	// a pass over a journal that outlives every archive: an object deleted *before* the archive
	// was taken is not in the archive, so nothing has to be kept out on its account. What has to
	// be kept out is what was deleted between the archive and now, which is exactly this window.
	//
	// A callback rather than a slice, for the reason every other read here is one: a tenant that
	// has been emptying its trash for two years has a journal larger than the thing being
	// restored.
	DeletedSince(ctx context.Context, since time.Time, yield func(Deletion) error) error
}

// Deletion is one entry of the journal.
type Deletion struct {
	// Entity is the table the row was removed from - the schema's vocabulary, which is what the
	// journal is written in and what the archive's entities cross over from.
	Entity    string
	EntityID  shared.ID
	DeletedAt time.Time
	Reason    string
}

// Schedules stores what runs when (E-05, backup-restore.md §5).
type Schedules interface {
	// Insert writes a schedule, with the moment it is next due already decided: the rule is
	// expanded by the use case that created it, not by every read afterwards.
	Insert(ctx context.Context, schedule domain.Schedule, nextRunAt time.Time) error

	// List answers the schedules visible in the caller's scope, oldest first.
	List(ctx context.Context) ([]domain.Schedule, error)

	// Find answers one schedule, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Schedule, error)

	// Due answers the schedules whose moment has come, earliest first. Bounded, because a
	// backlog of a thousand missed moments must not become a thousand jobs enqueued in one
	// transaction.
	Due(ctx context.Context, now time.Time, batch int) ([]domain.Schedule, error)

	// NextDue is the earliest moment anything in scope is owed, and the zero time when nothing
	// is. It is what a poller reschedules itself to, so that a quiet tenant costs one sleeping
	// row rather than a wake-up a minute.
	NextDue(ctx context.Context) (time.Time, error)

	// SetNextRun records when the schedule is next owed. The zero time clears it, which is what a
	// rule that has run out of occurrences leaves behind.
	SetNextRun(ctx context.Context, id shared.ID, nextRunAt time.Time) error
}

// Runs stores what happened (E-05).
type Runs interface {
	// Start writes the run and answers whether it got the target.
	//
	// False is not an error: §5 asks for a lock against two runs on one target, and a caller that
	// asked for a second one is asking for something that is already happening. The lock is the
	// statement rather than a check this method ran a moment earlier.
	Start(ctx context.Context, run domain.Run) (bool, error)

	// Find answers one run, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Run, error)

	// Finish records how a run ended and what it left behind. It is refused when the run is no
	// longer RUNNING, which is what makes a cancelled run stay cancelled.
	Finish(ctx context.Context, outcome domain.Outcome) error

	// LatestSuccessful is the archive an incremental continues: the newest run at this target
	// that finished and left something behind. ErrNotFound when there is none, which is what
	// turns a first incremental into a refusal rather than a chain with no root.
	LatestSuccessful(ctx context.Context, targetID shared.ID) (domain.Run, error)

	// RecordVerification writes down what `:verify` found.
	RecordVerification(ctx context.Context, id shared.ID, at time.Time, ok bool) error

	// SetExpiry records when the generation plan expects an archive to go, and clears it for one
	// the plan now intends to keep.
	SetExpiry(ctx context.Context, id shared.ID, expiresAt time.Time) error

	// MarkExpired moves a run whose archive has been deleted to EXPIRED.
	MarkExpired(ctx context.Context, id shared.ID) error

	// LastSuccessPerTarget is the number alert A-12 watches: when each target last had a backup
	// that worked. A target that has never had one is absent rather than zero - a gauge of zero
	// reads as 1970 on every dashboard.
	LastSuccessPerTarget(ctx context.Context) (map[shared.ID]time.Time, error)
}

// SealedCredential is a target's credential as the re-seal reads it: which row, and the sealed
// value. Nothing else of the target travels with it, FindBackupTargetCredential's reasoning.
type SealedCredential struct {
	TargetID   shared.ID
	Credential crypto.Sealed
}

// CredentialSealings is the re-seal's view of the targets (ADR-0045).
type CredentialSealings interface {
	// SealedNotUnder answers the credentials sealed under a key other than keyID.
	SealedNotUnder(ctx context.Context, keyID string) ([]SealedCredential, error)

	// Rewrap writes the moved wrapping, guarded by the key the row named when it was read. False
	// means the row changed in between.
	Rewrap(ctx context.Context, id shared.ID, sealed crypto.Sealed, expectedKeyID string) (bool, error)
}
