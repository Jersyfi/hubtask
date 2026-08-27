// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// archiveKeyBytes is the length of the key a stream cipher wants. AES-256, as §4 requires.
const archiveKeyBytes = 32

// archiveKeyPurpose binds a target's archive key to that target, so that two targets never share
// one and a key lifted from one installation's configuration cannot open another's archives.
func archiveKeyPurpose(targetID shared.ID) crypto.Purpose {
	return crypto.Purpose("backup_target.archive:" + targetID.String())
}

// Performer runs one backup, end to end (E-05, backup-restore.md §5).
//
// It is the application layer's half of the `backup.run` job: the worker owns the queue, the
// retries and the lease, and everything about what a backup *is* lives here.
type Performer struct {
	Runs      repository.Runs
	Targets   repository.Targets
	Export    repository.Export
	Opener    backupstorage.Opener
	Encryptor crypto.Encryptor
	Keys      crypto.KeyMaterialiser
	Cipher    crypto.StreamCipher
	Objects   storage.ObjectStore
	// Snapshot is the consistency §5 requires: the export reads one point in time rather than a
	// mixture of before and after.
	Snapshot   persistence.Snapshot
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// SchemaVersion and ProductVersion go into the manifest. They are configuration rather than
	// something read at run time, because what an archive has to record is the build that wrote
	// it, and a build knows that about itself.
	SchemaVersion  string
	ProductVersion string
}

// PerformInput is one run, as the job's payload describes it.
type PerformInput struct {
	RunID        shared.ID
	TargetID     shared.ID
	TenantID     shared.ID
	ScheduleID   shared.ID
	ParentRunID  shared.ID
	Mode         domain.Mode
	Trigger      domain.Trigger
	IncludeMedia bool
	IncludeAudit bool
	// Report is how far along the run is, between 0 and 1. It may be nil, and losing a progress
	// reading is never a reason to fail a backup.
	Report func(fraction float64)
}

// Perform writes the archive and records what it left behind.
//
// The shape is three acts, and the middle one is long. First a short transaction that claims the
// target and reads what the run needs; then the export, inside a REPEATABLE READ snapshot, writing
// to somebody else's machine; then a short transaction that records the outcome.
//
// The middle act holds a database transaction across calls to a backup target, which
// observability-reliability.md §8 forbids everywhere else. §5 requires it here in as many words -
// an archive that is a mixture of before and after is not a backup - and three things make it
// survivable: the job runs on the worker role rather than the API path, it is Detached so the
// runner's own transaction is not also open, and the snapshot is read-only so it blocks nothing but
// vacuum. Resolving the media locations inside the same snapshot is a second reason to hold it: the
// mapping from a content address to a storage key is then the one the snapshot saw, and cannot
// change under the run.
func (p Performer) Perform(ctx context.Context, in PerformInput) (domain.Run, error) {
	ready, err := p.claim(ctx, in)
	if err != nil {
		return domain.Run{}, err
	}

	manifest, snapshotAt, err := p.write(ctx, in, ready)
	if err != nil {
		// A run that failed is recorded as failed rather than left RUNNING: a row nobody closed
		// would hold the target's lock until somebody noticed, and the code is what an operator
		// reads on the dashboard.
		if closing := p.fail(ctx, in, err); closing != nil {
			return domain.Run{}, closing
		}
		return domain.Run{}, err
	}
	return p.succeed(ctx, in, ready.prefix, manifest, snapshotAt)
}

// prepared is everything the export needs, read once.
type prepared struct {
	target     domain.Target
	store      backupstorage.Store
	key        crypto.MasterDerived
	prefix     string
	since      time.Time
	ancestors  []string
	parentPath string
	startedAt  time.Time
}

// claim takes the target and reads the run's surroundings, in one short transaction.
func (p Performer) claim(ctx context.Context, in PerformInput) (prepared, error) {
	now := p.Clock.Now()
	out := prepared{startedAt: now}

	err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		target, err := p.Targets.Find(ctx, in.TargetID)
		if err != nil {
			return err
		}
		out.target = target

		claimed, err := p.Runs.Start(ctx, domain.Run{
			ID: in.RunID, ScheduleID: in.ScheduleID, TargetID: in.TargetID, TenantID: in.TenantID,
			ParentRunID: in.ParentRunID, Trigger: in.Trigger, Mode: in.Mode,
			Status: domain.RunRunning, StartedAt: now,
		})
		if err != nil {
			return err
		}
		if !claimed {
			// Another run holds the target. Not an error the job should retry into: the work the
			// caller asked for is happening, and a second archive at the same moment is what the
			// lock exists to prevent.
			return shared.ErrConflict.WithDetail(domain.CodeTargetBusy).
				WithParams(map[string]string{"target_id": in.TargetID.String()})
		}

		if in.Mode == domain.ModeIncremental {
			parent, err := p.Runs.Find(ctx, in.ParentRunID)
			if err != nil {
				return err
			}
			out.since, out.parentPath = parent.SnapshotAt, parent.ArchivePath
			out.ancestors, err = p.chainOf(ctx, parent)
			if err != nil {
				return err
			}
		}

		credentials, err := p.credentialsOf(ctx, target)
		if err != nil {
			return err
		}
		store, err := p.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		if err != nil {
			return err
		}
		out.store = store
		return nil
	})
	if err != nil {
		return prepared{}, err
	}

	if out.target.EncryptionMode == domain.EncryptionAES256GCM {
		out.key, err = p.Keys.DeriveFromMaster(ctx, archiveKeyPurpose(in.TargetID), archiveKeyBytes)
		if err != nil {
			return prepared{}, err
		}
	}
	out.prefix = archive.Name(in.TenantID, now, modeOf(in.Mode))
	return out, nil
}

// chainOf walks back from the parent to the full archive, newest first, so that the writer knows
// where the chain already holds a medium.
func (p Performer) chainOf(ctx context.Context, parent domain.Run) ([]string, error) {
	var prefixes []string
	current := parent
	for range maxChainLength {
		if current.ArchivePath != "" {
			prefixes = append(prefixes, current.ArchivePath)
		}
		if current.Mode == domain.ModeFull || current.ParentRunID.IsZero() {
			return prefixes, nil
		}
		next, err := p.Runs.Find(ctx, current.ParentRunID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// A chain whose parent has been expired away is a chain that cannot be
				// continued. The next run has to be a full one, and saying so now is better than
				// writing an incremental nobody can restore.
				return nil, shared.ErrConflict.WithDetail(domain.CodeChainIncomplete).
					WithParams(map[string]string{"run_id": current.ID.String()})
			}
			return nil, err
		}
		current = next
	}
	return nil, shared.ErrConflict.WithDetail(domain.CodeChainIncomplete).
		WithParams(map[string]string{"run_id": parent.ID.String()})
}

// maxChainLength bounds the walk. A chain this long is a schedule that has never run a full backup,
// which is a configuration to fix rather than a chain to follow - and the bound is also what stops
// a row whose parent points at itself becoming a loop.
const maxChainLength = 512

// write is the export: the snapshot, and the archive that comes out of it.
func (p Performer) write(
	ctx context.Context, in PerformInput, ready prepared,
) (archive.Manifest, time.Time, error) {
	var manifest archive.Manifest
	var snapshotAt time.Time

	err := p.Snapshot.WithinSnapshot(ctx, persistence.Scope{TenantID: in.TenantID},
		func(snapshotCtx context.Context, at time.Time) error {
			snapshotAt = at
			source := ExportSource{Export: p.Export, SnapshotAt: at}
			media := ExportMedia{Export: p.Export, Objects: p.Objects}

			request := archive.Request{
				ArchiveID: in.RunID, Prefix: ready.prefix,
				Scope:          archive.Scope{Kind: archive.ScopeTenant, ID: in.TenantID.String()},
				Mode:           modeOf(in.Mode),
				SnapshotAt:     at,
				Since:          ready.since,
				ParentID:       in.ParentRunID.String(),
				ParentPrefix:   ready.parentPath,
				Ancestors:      ready.ancestors,
				SchemaVersion:  p.SchemaVersion,
				ProductVersion: p.ProductVersion,
				Encryption:     encryptionOf(ready),
				Key:            ready.key.Key,
				IncludeMedia:   in.IncludeMedia,
				IncludeAudit:   in.IncludeAudit,
			}

			written, err := archive.NewWriter(ready.store, p.Cipher, media).
				Write(snapshotCtx, request, newProgress(source, in.Report))
			if err != nil {
				return err
			}
			manifest = written
			return nil
		})
	if err != nil {
		return archive.Manifest{}, time.Time{}, err
	}
	return manifest, snapshotAt, nil
}

// progressing wraps the source and counts the entities as they go past, which is the only honest
// progress a backup can report: how many rows a tenant has is a question that costs a pass over
// the tenant to answer, and paying for it would double the run to draw a bar.
type progressing struct {
	source archive.Source
	report func(float64)
	done   int
}

func (p *progressing) Records(
	ctx context.Context, entity archive.Entity, since time.Time, yield func(archive.Record) error,
) error {
	err := p.source.Records(ctx, entity, since, yield)
	p.done++
	if p.report != nil {
		p.report(float64(p.done) / float64(len(archive.Entities())))
	}
	return err
}

// progressing has to be addressable to count, so the writer is handed a pointer.
func newProgress(source archive.Source, report func(float64)) archive.Source {
	return &progressing{source: source, report: report}
}

func encryptionOf(ready prepared) archive.Encryption {
	if ready.target.EncryptionMode != domain.EncryptionAES256GCM {
		return archive.Encryption{Mode: archive.EncryptionNone}
	}
	return archive.Encryption{Mode: archive.EncryptionAES256GCM, KeyID: ready.key.KeyID}
}

// succeed records what the run left behind.
func (p Performer) succeed(
	ctx context.Context, in PerformInput, prefix string, manifest archive.Manifest, snapshotAt time.Time,
) (domain.Run, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return domain.Run{}, shared.Internalf("backup: recording a manifest: %w", err)
	}
	var items int64
	for _, count := range manifest.Counts {
		items += count
	}
	var bytes int64
	for _, file := range manifest.Files {
		bytes += file.Bytes
	}

	outcome := domain.Outcome{
		ID: in.RunID, Status: domain.RunSucceeded,
		ArchivePath: prefix, Manifest: encoded,
		SizeBytes: bytes + manifest.MediaBytes, ItemCount: int(items),
		MediaCount: int(manifest.MediaCount), Checksum: manifest.ArchiveID,
		SnapshotAt: snapshotAt, FinishedAt: p.Clock.Now(),
	}

	var run domain.Run
	err = p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		if err := p.Runs.Finish(ctx, outcome); err != nil {
			return err
		}
		var err error
		run, err = p.Runs.Find(ctx, in.RunID)
		return err
	})
	if err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

// Abandon closes a run whose job the queue has given up on (#207).
//
// A row left RUNNING by a worker that is not coming back holds the one-run-per-target lock for
// ever; this closes it as FAILED under its own code, which frees the target and puts the truth
// where GetBackupRun reads. A run that never claimed its row, or that is terminal already, answers
// a conflict - the same "nothing to close" this treats as done, because both mean nobody is
// waiting on the row.
func (p Performer) Abandon(ctx context.Context, runID, tenantID shared.ID) error {
	err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		return p.Runs.Finish(ctx, domain.Outcome{
			ID: runID, Status: domain.RunFailed,
			FinishedAt: p.Clock.Now(), ErrorCode: domain.CodeRunAbandoned,
		})
	})
	if err != nil && !errors.Is(err, shared.ErrConflict) && !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	return nil
}

// fail closes a run that did not work, with the code and nothing else.
//
// The code and never a message: an error's text can carry a bucket name, a host or a path, and a
// dashboard is a place a lot of people look (rules 8 and 10).
func (p Performer) fail(ctx context.Context, in PerformInput, cause error) error {
	// A run that never claimed the target left no row to close, and a conflict is exactly that.
	if errors.Is(cause, shared.ErrConflict) {
		var domainErr *shared.Error
		if errors.As(cause, &domainErr) && domainErr.DetailCode == domain.CodeTargetBusy {
			return nil
		}
	}

	outcome := domain.Outcome{
		ID: in.RunID, Status: domain.RunFailed,
		FinishedAt: p.Clock.Now(), ErrorCode: runFailureCode(cause),
	}
	err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		return p.Runs.Finish(ctx, outcome)
	})
	if err != nil && !errors.Is(err, shared.ErrConflict) {
		// A run that is no longer RUNNING was cancelled, and a cancelled run stays cancelled.
		return err
	}
	return nil
}

// runFailureCode is the message code of a failure, and `backup.run_failed` for anything
// unclassified - never the text, which can carry a host or a path.
func runFailureCode(err error) string {
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "" {
		return domainErr.DetailCode
	}
	return domain.CodeRunFailed
}

// modeOf translates the domain's mode into the archive's. Two enums with the same two values, kept
// apart because one is a schedule setting and the other is a wire format.
func modeOf(mode domain.Mode) archive.Mode {
	if mode == domain.ModeFull {
		return archive.ModeFull
	}
	return archive.ModeIncremental
}

// credentialsOf opens a target's stored credential. It is the second caller of the unsealing the
// probe uses, and the only other one: a credential becomes readable to open a connection, and for
// nothing else.
func (p Performer) credentialsOf(
	ctx context.Context, target domain.Target,
) (map[string]secret.Secret, error) {
	sealed, err := p.Targets.Credential(ctx, target.ID)
	if err != nil {
		return nil, err
	}
	return unsealCredentials(ctx, p.Encryptor, target.ID, sealed)
}

// Verify checks one archive at its target without restoring it, and writes down what it found
// (E-05, backup-restore.md §3).
//
// The answer is recorded whichever way it came out. "This archive is damaged" is the finding the
// endpoint exists to produce, and a run whose verification failed and recorded nothing would look
// exactly like one nobody had checked.
func (p Performer) Verify(ctx context.Context, runID, tenantID shared.ID) (bool, error) {
	var (
		run   domain.Run
		store backupstorage.Store
	)
	err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		var err error
		run, err = p.Runs.Find(ctx, runID)
		if err != nil {
			return err
		}
		if !run.Succeeded() || run.ArchivePath == "" {
			return shared.ErrConflict.WithDetail(domain.CodeRunHasNoArchive).
				WithParams(map[string]string{"run_id": runID.String(), "status": string(run.Status)})
		}

		target, err := p.Targets.Find(ctx, run.TargetID)
		if err != nil {
			return err
		}
		credentials, err := p.credentialsOf(ctx, target)
		if err != nil {
			return err
		}
		store, err = p.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		return err
	})
	if err != nil {
		return false, err
	}

	// Outside a transaction: reading every member of an archive over somebody else's network is
	// minutes, and §8's rule about holding a connection across an external call has no exception
	// here - nothing about verifying needs a consistent view of the database.
	sound := archive.NewReader(store, p.Cipher).Verify(ctx, run.ArchivePath) == nil

	recorded := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		return p.Runs.RecordVerification(ctx, runID, p.Clock.Now(), sound)
	})
	if recorded != nil {
		return sound, recorded
	}
	return sound, nil
}

// ExpireInput is one retention pass: which target, under which plan.
type ExpireInput struct {
	TargetID shared.ID
	TenantID shared.ID
	Plan     domain.Retention
	// TimeZone is the zone the generations are counted in - the schedule's, because a day is a
	// day where the operator is. UTC when a run had no schedule behind it.
	TimeZone string
}

// Expire applies the generation plan to what is at the target (backup-restore.md §6, BK-8).
//
// It runs only after a successful backup, and that is §6's rule rather than an ordering choice: a
// failed run means the newest archive is not the one the plan thinks it is, and deleting against a
// stale reading is how a target ends up with nothing.
//
// What is at the target is read from the manifests rather than from the database, which is the
// other half of the same rule. A row that says an archive exists is a row; the archive is what is
// at the target, and expiry deletes files. Anything under the prefix that is not a Hubtask archive
// is not listed and therefore cannot be deleted - "other files at the target stay untouched" is not
// a check here, it is the absence of a code path.
func (p Performer) Expire(ctx context.Context, in ExpireInput) (domain.Expiry, error) {
	var store backupstorage.Store
	byPath := map[string]domain.Run{}

	err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		target, err := p.Targets.Find(ctx, in.TargetID)
		if err != nil {
			return err
		}
		credentials, err := p.credentialsOf(ctx, target)
		if err != nil {
			return err
		}
		store, err = p.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		return err
	})
	if err != nil {
		return domain.Expiry{}, err
	}

	reader := archive.NewReader(store, p.Cipher)
	described, err := reader.List(ctx, "")
	if err != nil {
		return domain.Expiry{}, err
	}

	scope := archive.Prefix(in.TenantID)
	archives := make([]domain.Archive, 0, len(described))
	for _, description := range described {
		// Somebody else's archives at a shared target are not this plan's to count or delete. The
		// name is the filter, because the storage port's prefix is a place rather than a string
		// and an archive's name is a directory under the target's root.
		if !strings.HasPrefix(description.Prefix, scope) {
			continue
		}
		if !description.Complete {
			// A run that is still going, or one that died. Neither is something a plan gets to
			// count or delete: the first is in progress and the second has no manifest anybody
			// should trust.
			continue
		}
		id, err := shared.ParseID(description.Manifest.ArchiveID)
		if err != nil {
			continue
		}
		parent, _ := shared.ParseID(description.Manifest.ParentID)
		archives = append(archives, domain.Archive{
			ID: id, TakenAt: description.Manifest.SnapshotAt,
			Mode: modeBack(description.Manifest.Mode), ParentID: parent,
		})
		byPath[description.Prefix] = domain.Run{ID: id}
	}

	zone := zoneOr(in.TimeZone)
	expiry := in.Plan.Apply(archives, zone)

	for _, going := range expiry.Expire {
		prefix := pathOf(byPath, going.ID)
		if prefix == "" {
			continue
		}
		if err := deleteArchive(ctx, store, prefix); err != nil {
			// A target that will not let an archive go is a notice rather than a failure: §6 says
			// so for object lock in particular, and retrying forever against a WORM bucket is the
			// behaviour it exists to forbid. The run stays SUCCEEDED and the next pass will meet
			// it again.
			return expiry, err
		}
		if err := p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID},
			func(ctx context.Context) error { return p.Runs.MarkExpired(ctx, going.ID) }); err != nil {
			return expiry, err
		}
	}
	return expiry, nil
}

// deleteArchive removes every object of one archive, and nothing that is not one of them.
//
// The listing is of the archive's own prefix, so what is deleted is what the archive is made of.
// `checksums.txt` goes first, deliberately: it is the file that says the archive is complete, so an
// interrupted deletion leaves something that reads as unfinished rather than as sound.
func deleteArchive(ctx context.Context, store backupstorage.Store, prefix string) error {
	if err := store.Delete(ctx, prefix+"/"+archive.ChecksumsName); err != nil {
		return err
	}
	entries, err := store.List(ctx, prefix+"/")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := store.Delete(ctx, entry.Key); err != nil {
			return err
		}
	}
	return nil
}

func pathOf(byPath map[string]domain.Run, id shared.ID) string {
	for prefix, run := range byPath {
		if run.ID == id {
			return prefix
		}
	}
	return ""
}

func modeBack(mode archive.Mode) domain.Mode {
	if mode == archive.ModeFull {
		return domain.ModeFull
	}
	return domain.ModeIncremental
}

// zoneOr reads a time zone, falling back to UTC. A zone this installation does not know is not
// worth failing a retention pass for - the generations shift by hours, and refusing to delete
// anything because tzdata is old is the worse outcome.
func zoneOr(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return zone
}

// RetentionExport is the archive a retention rule writes before it removes anything
// (data-retention.md §6, E-07).
//
// It sits here rather than in the lifecycle context because writing an archive is this context's
// work, and the seam the retention engine declares is one method wide. What it does is a full
// backup run of the tenant, with a trigger of its own: "who wrote this archive, and why" is the
// question `backup_run.trigger` exists to answer, and an export the machinery took before deleting
// is not a run anybody asked for.
type RetentionExport struct {
	Performer Performer
	IDs       clock.IDGenerator
}

// Export writes one archive of the tenant to the target and answers the run it produced.
func (e RetentionExport) Export(ctx context.Context, targetID shared.ID) (shared.ID, error) {
	scope, ok := e.Performer.UnitOfWork.(persistence.ScopeSource)
	if !ok {
		return "", shared.Internalf("backup: the unit of work cannot say which tenant it is in")
	}
	current, found := scope.ScopeFromContext(ctx)
	if !found || current.TenantID.IsZero() {
		return "", shared.Internalf("backup: a retention export outside a tenant's transaction")
	}

	runID := e.IDs.NewID()
	run, err := e.Performer.Perform(ctx, PerformInput{
		RunID: runID, TargetID: targetID, TenantID: current.TenantID,
		Mode: domain.ModeFull, Trigger: domain.TriggerPreDelete,
		IncludeMedia: true, IncludeAudit: true,
	})
	if err != nil {
		return "", err
	}
	return run.ID, nil
}
