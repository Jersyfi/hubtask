// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SafetyBackup takes the copy §8.3 step 4 asks for before a destructive mode.
//
// An interface rather than the performer itself, so that the restore can be tested without a
// backup target underneath it - and so that the ordering the step is about, the copy *before* the
// destruction, is visible as a call rather than buried in a struct.
type SafetyBackup interface {
	Perform(ctx context.Context, in PerformInput) (domain.Run, error)
}

// Applier performs one restore, end to end (E-06, backup-restore.md §8.3).
//
// The procedure of §8.3 is a checklist and this follows it in order: pre-check, dry run with a
// report, the confirmation and the step-up (which the use case has already refused without), the
// safety copy, execution in batches, and then the follow-up. Each step is a method, and the order
// they are called in is the order the document lists them.
type Applier struct {
	Restores  repository.Restores
	Targets   repository.Targets
	Import    repository.Import
	Journal   repository.Journal
	Opener    backupstorage.Opener
	Encryptor crypto.Encryptor
	Keys      crypto.KeyMaterialiser
	Cipher    crypto.StreamCipher
	Objects   storage.ObjectStore
	// Safety takes the copy before a destructive mode. Optional: an installation with no target
	// to write it to refuses the destructive mode rather than proceeding without the copy.
	Safety     SafetyBackup
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// SchemaVersion is the migration this build stands at. An archive from a newer one is refused
	// rather than half-read: a restore migrates upwards and cannot go the other way.
	SchemaVersion string
	// Batch is how many records one transaction writes. §8.3 asks for "rollback within a
	// transaction per batch size", which is what this is: a cancelled restore loses the batch in
	// flight and keeps everything before it.
	Batch int
}

// DefaultRestoreBatch is how many records one transaction writes unless the installation says
// otherwise. Large enough that a tenant is not written in thousands of transactions, small enough
// that a cancellation costs a fraction of a second's work.
const DefaultRestoreBatch = 200

// ApplyInput is one restore, as the job's payload describes it.
type ApplyInput struct {
	RestoreID shared.ID
	// TenantID is the tenant that asked, which is the one the run row lives in.
	TenantID shared.ID
	// Report is how far along the restore is, between 0 and 1. It may be nil, and losing a
	// progress reading is never a reason to fail a restore.
	Report func(fraction float64)
}

// Apply runs one restore and records what it did.
//
// The outcome is recorded whichever way it came out, including a refusal: a run left RUNNING would
// hold the tenant's restore lock until somebody noticed, and "what happened to the restore I
// started" is a question the row has to answer.
func (a Applier) Apply(ctx context.Context, in ApplyInput) (domain.Report, error) {
	ready, err := a.claim(ctx, in)
	if err != nil {
		return domain.Report{}, err
	}

	report, err := a.run(ctx, in, ready)
	if err != nil {
		// The report goes in with the failure. A restore that got through half the archive and
		// then lost its worker has an account of that half, and throwing it away would make the
		// next attempt's report describe a fraction of what was done.
		if closing := a.fail(ctx, in, report, err); closing != nil {
			return domain.Report{}, closing
		}
		return domain.Report{}, err
	}
	return report, a.succeed(ctx, in, report, ready.safetyRunID)
}

// claimed is everything the restore needs, read once.
type claimed struct {
	restore domain.Restore
	store   backupstorage.Store
	// into is the tenant the rows land in: the target tenant, which differs from the asking one
	// only for NEW_TENANT.
	into        persistence.Scope
	safetyRunID shared.ID
}

// claim takes the tenant's restore lock and reads the run's surroundings, in one short
// transaction.
func (a Applier) claim(ctx context.Context, in ApplyInput) (claimed, error) {
	var out claimed
	asking := persistence.Scope{TenantID: in.TenantID}

	err := a.UnitOfWork.Within(ctx, asking, func(ctx context.Context) error {
		got, err := a.Restores.Claim(ctx, in.RestoreID, a.Clock.Now())
		if err != nil {
			return err
		}
		if !got {
			// Another restore holds the tenant, or this one is already over. Neither is a failure
			// to retry into: the work the caller asked for is either happening or finished.
			return shared.ErrConflict.WithDetail(domain.CodeRestoreTargetBusy).
				WithParams(map[string]string{"restore_id": in.RestoreID.String()})
		}

		restore, err := a.Restores.Find(ctx, in.RestoreID)
		if err != nil {
			return err
		}
		out.restore = restore
		out.safetyRunID = restore.SafetyRunID

		target, err := a.Targets.Find(ctx, restore.TargetID)
		if err != nil {
			return err
		}
		sealed, err := a.Targets.Credential(ctx, restore.TargetID)
		if err != nil {
			return err
		}
		credentials, err := unsealCredentials(ctx, a.Encryptor, restore.TargetID, sealed)
		if err != nil {
			return err
		}
		out.store, err = a.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		return err
	})
	if err != nil {
		return claimed{}, err
	}

	// The tenant the rows land in. For NEW_TENANT it is one that did not exist when the restore
	// was asked for, and the elevation is safe because the identifier was minted by the use case
	// rather than supplied by the caller: there is nothing of anybody else's under it.
	out.into = persistence.Scope{TenantID: out.restore.TenantID}
	if out.restore.TenantID.IsZero() {
		out.into = asking
	}
	return out, nil
}

// run is §8.3 in order.
func (a Applier) run(ctx context.Context, in ApplyInput, ready claimed) (domain.Report, error) {
	reader := archive.NewReader(ready.store, a.Cipher)

	chain, key, err := a.precheck(ctx, reader, ready.restore, in.TenantID)
	if err != nil {
		return domain.Report{}, err
	}

	plan := plan{
		restore: ready.restore, chain: chain, key: key,
		reader: reader, scope: ready.into, asker: in.TenantID, report: in.Report,
		// NEW_TENANT copies a workspace whose rows still live in this installation, and every
		// identity in the schema is a global one - so the copy derives a new identity for every
		// row and follows the references, the way DUPLICATE does for a collision. Without this the
		// first insert collides with the source (#206).
		remapAll: ready.restore.Mode == domain.RestoreNewTenant,
	}

	// A dry run and an INSPECT are the same thing said twice: INSPECT is the mode that cannot
	// write, and `dry_run` is the flag that says not to. Either way nothing below opens a writing
	// transaction, which is what makes "changes nothing" a property of the code rather than a
	// promise.
	plan.dry = ready.restore.DryRun || !ready.restore.Mode.Writes()

	if !plan.dry && ready.restore.Mode == domain.RestoreNewTenant {
		if err := a.assertFresh(ctx, ready.into); err != nil {
			return domain.Report{}, err
		}
	}
	if !plan.dry && ready.restore.Mode.Destructive() {
		if err := a.takeSafetyCopy(ctx, in, &ready); err != nil {
			return domain.Report{}, err
		}
	}
	return a.apply(ctx, plan)
}

// precheck is §8.3 step 1: is this archive this tenant's, can this build read it, and is the chain
// complete.
//
// Every one of these is a refusal rather than a warning. A restore that starts and stops half way
// through leaves a tenant in a state that was never a state - and the pre-check is cheap: it reads
// manifests, not data.
func (a Applier) precheck(
	ctx context.Context, reader *archive.Reader, restore domain.Restore, asking shared.ID,
) ([]archive.Description, secret.Bytes, error) {
	chain, err := reader.Chain(ctx, restore.SourceArchive)
	if err != nil {
		return nil, secret.Bytes{}, err
	}
	newest := chain[0]

	// INSTANCE has nothing to restore yet (0.6.0): no writer produces an instance-scoped archive,
	// and a tenant archive under the INSTANCE mode would be an approximation §8's table does not
	// allow. backup-restore.md §8 says the scope check refuses the mode; it used to fall out of
	// the comparison below by accident, and now it is said.
	if restore.Mode == domain.RestoreInstance {
		return nil, secret.Bytes{}, shared.ErrValidation.
			WithDetail(domain.CodeRestoreArchiveScopeMismatch).
			WithParams(map[string]string{"archive": restore.SourceArchive})
	}

	// BK-10 at the dry run and at the execution, not only at the listing. The archive's scope is
	// in its manifest, and the manifest is the one member nobody can forge without the target's
	// credentials.
	//
	// The manifest is compared against the tenant that *asked* - the one the run row lives in -
	// because BK-10 protects the archive's owner, and the owner question is the same in every
	// mode. Where the rows land is a separate question: for every mode but NEW_TENANT it is the
	// asker itself (StartRestore refuses any other), and for NEW_TENANT it is an identifier the
	// use case minted a moment ago, guarded by assertFresh below. Comparing against the
	// destination instead is the defect #206 records: a NEW_TENANT restore could never match its
	// own archive.
	if newest.Manifest.Scope.Kind != archive.ScopeTenant || newest.Manifest.Scope.ID != asking.String() {
		return nil, secret.Bytes{}, shared.ErrValidation.
			WithDetail(domain.CodeRestoreArchiveScopeMismatch).
			WithParams(map[string]string{"archive": restore.SourceArchive})
	}

	// An archive from a newer schema than this build has migrated to. A restore reads JSON Lines
	// and migrates upwards; downwards it cannot go, and guessing which columns a future migration
	// added is how a restore writes a row that is silently wrong (§3).
	if newest.Manifest.SchemaVersion > a.SchemaVersion {
		return nil, secret.Bytes{}, shared.ErrConflict.WithDetail(domain.CodeRestoreSchemaAhead).
			WithParams(map[string]string{
				"archive": newest.Manifest.SchemaVersion, "installed": a.SchemaVersion,
			})
	}

	// Decryptability, before anything is read. An archive that cannot be opened is worth finding
	// out about now rather than after the first batch has landed.
	var key secret.Bytes
	if newest.Manifest.Encryption.IsEncrypted() {
		key, err = a.Keys.ReproduceFromMaster(ctx,
			newest.Manifest.Encryption.KeyID, archiveKeyPurpose(restore.TargetID), archiveKeyBytes)
		if err != nil {
			return nil, secret.Bytes{}, err
		}
	}
	return chain, key, nil
}

// assertFresh refuses a NEW_TENANT restore whose destination already exists.
//
// The mode's whole safety argument is that its destination was minted by the use case a moment
// ago, so nothing of anybody else's can be under it. A run row naming a living tenant - however it
// came to - is a row that argument no longer covers, and the honest outcome is a refusal before
// the first row is written rather than a write into somebody's workspace. The read runs in the
// destination's own scope, where row level security lets a tenant see exactly its own row: a
// tenant that does not exist answers nothing, which is the answer this wants.
func (a Applier) assertFresh(ctx context.Context, into persistence.Scope) error {
	var held bool
	err := a.UnitOfWork.WithinReadOnly(ctx, into, func(ctx context.Context) error {
		var err error
		held, err = a.Import.Holds(ctx, tenantTable, map[string]any{"id": into.TenantID.String()})
		return err
	})
	if err != nil {
		return err
	}
	if held {
		return shared.ErrConflict.WithDetail(domain.CodeRestoreTenantNotNew)
	}
	return nil
}

// takeSafetyCopy is §8.3 step 4: a copy of the current state before a destructive mode.
//
// Refused rather than skipped when there is nowhere to write it. A destructive restore with no way
// back is the situation the step exists to prevent, and "the target was full" is not a reason to
// proceed - it is the reason to stop.
func (a Applier) takeSafetyCopy(ctx context.Context, in ApplyInput, ready *claimed) error {
	if !ready.restore.CreateSafetyBackup {
		return nil
	}
	if !ready.safetyRunID.IsZero() {
		// A resumed restore has already taken one. Taking a second would copy the half-restored
		// state, which is the one state nobody wants a copy of (BK-7).
		return nil
	}
	if a.Safety == nil {
		return shared.ErrConflict.WithDetail(domain.CodeRestoreSafetyCopyUnavailable).
			WithParams(map[string]string{"restore_id": in.RestoreID.String()})
	}

	runID := a.IDs.NewID()
	run, err := a.Safety.Perform(ctx, PerformInput{
		RunID: runID, TargetID: ready.restore.TargetID, TenantID: ready.into.TenantID,
		Mode: domain.ModeFull, Trigger: domain.TriggerPreRestore,
		IncludeMedia: true, IncludeAudit: true,
	})
	if err != nil {
		return err
	}

	// Recorded before the mode runs, so the way back is findable from the run even if the run
	// then fails.
	if err := a.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID},
		func(ctx context.Context) error {
			return a.Restores.RecordSafetyCopy(ctx, in.RestoreID, run.ID)
		}); err != nil {
		return err
	}
	ready.safetyRunID = run.ID
	return nil
}

// succeed records what the restore did.
func (a Applier) succeed(
	ctx context.Context, in ApplyInput, report domain.Report, safetyRunID shared.ID,
) error {
	return a.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		return a.Restores.Finish(ctx, domain.RestoreOutcome{
			ID: in.RestoreID, Status: domain.RestoreSucceeded, Report: report,
			SafetyRunID: safetyRunID, FinishedAt: a.Clock.Now(),
		})
	})
}

// Abandon closes a restore whose job the queue has given up on (#207).
//
// The open row - PENDING or RUNNING - holds the one-restore-per-tenant lock: InProgress refuses
// every later restore while it stands, and the worker that would have closed it is not coming
// back. Closed as FAILED under its own code; a row already terminal answers a conflict this
// treats as done.
func (a Applier) Abandon(ctx context.Context, restoreID, tenantID shared.ID) error {
	err := a.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		return a.Restores.Finish(ctx, domain.RestoreOutcome{
			ID: restoreID, Status: domain.RestoreFailed,
			FinishedAt: a.Clock.Now(), ErrorCode: domain.CodeRestoreAbandoned,
		})
	})
	if err != nil && !errors.Is(err, shared.ErrConflict) && !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	return nil
}

// fail closes a restore that did not work, with the code and nothing else.
//
// The code and never a message: an error's text can carry a bucket name, a host or a path, and a
// dashboard is a place a lot of people look (rules 8 and 10).
func (a Applier) fail(ctx context.Context, in ApplyInput, report domain.Report, cause error) error {
	// A restore that never got the lock left no row of its own to close.
	var domainErr *shared.Error
	if errors.As(cause, &domainErr) && domainErr.DetailCode == domain.CodeRestoreTargetBusy {
		return nil
	}

	err := a.UnitOfWork.Within(ctx, persistence.Scope{TenantID: in.TenantID}, func(ctx context.Context) error {
		return a.Restores.Finish(ctx, domain.RestoreOutcome{
			ID: in.RestoreID, Status: domain.RestoreFailed, Report: report,
			FinishedAt: a.Clock.Now(), ErrorCode: restoreFailureCode(cause),
		})
	})
	if err != nil && !errors.Is(err, shared.ErrConflict) {
		// A restore that is no longer RUNNING was cancelled, and a cancelled restore stays
		// cancelled.
		return err
	}
	return nil
}

// restoreFailureCode is the message code of a failure, and `backup.restore_failed` for anything
// unclassified - never the text, which can carry a host or a path.
func restoreFailureCode(err error) string {
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "" {
		return domainErr.DetailCode
	}
	return domain.CodeRestoreFailed
}

// plan is one restore's reading of the world, fixed before the first record is applied.
type plan struct {
	restore domain.Restore
	// chain is the archives to read, newest first, back to the full archive at the root.
	chain  []archive.Description
	key    secret.Bytes
	reader *archive.Reader
	scope  persistence.Scope
	// asker is the tenant that asked, which is where the run row lives. It differs from scope only
	// for NEW_TENANT.
	asker shared.ID
	// remapAll is the NEW_TENANT identity rule: every row with an identity of its own gets a
	// derived new one and every reference follows, because the source rows still live in this
	// installation and every identity in the schema is global. Derived rather than drawn, for the
	// reason DuplicateID gives: a resumed attempt has to produce the same identifiers.
	remapAll bool
	dry      bool
	report   func(float64)
}

// newest is the archive the restore represents: the one that was asked for.
func (p plan) newest() archive.Description { return p.chain[0] }

// asking is the scope of the tenant that asked for the restore, which is where the run row lives.
func (p plan) asking() persistence.Scope { return persistence.Scope{TenantID: p.asker} }

// apply reads the chain and writes what the mode says to write.
func (a Applier) apply(ctx context.Context, p plan) (domain.Report, error) {
	state, err := a.prepare(ctx, p)
	if err != nil {
		return domain.Report{}, err
	}

	if !p.dry && p.restore.Mode == domain.RestoreReplaceTenant {
		if err := a.emptyTenant(ctx, p); err != nil {
			return domain.Report{}, err
		}
	}

	entities := archive.RestoredEntities()
	for index, entity := range entities {
		if err := a.applyEntity(ctx, p, state, entity); err != nil {
			return state.report, err
		}
		if p.report != nil {
			p.report(float64(index+1) / float64(len(entities)))
		}
	}
	if err := state.flush(ctx); err != nil {
		return state.report, err
	}
	return state.report, nil
}

// emptyTenant is what REPLACE_TENANT is made of: the tenant is reset to the archive, so what the
// archive does not name goes.
//
// In reverse order, children before parents, so that a cascade never has to be relied on - a
// cascade is the database deciding what else should go, and "what else should go" is exactly the
// question a destructive mode must not answer implicitly.
func (a Applier) emptyTenant(ctx context.Context, p plan) error {
	entities := archive.RestoredEntities()
	slices.Reverse(entities)

	return a.UnitOfWork.Within(ctx, p.scope, func(ctx context.Context) error {
		for _, entity := range entities {
			// The tenant's own row is not emptied - it is the row the transaction is standing
			// inside - and it is overwritten by the archive's copy like everything else.
			if entity.Table == tenantTable {
				continue
			}
			if _, err := a.Import.Clear(ctx, entity.Table); err != nil {
				return err
			}
		}
		return nil
	})
}

const tenantTable = "tenant"

// state is what one restore accumulates while it reads: what it may not bring back, what it was
// asked for, what it has already given a new identity to, and the batch waiting to be written.
type state struct {
	applier Applier
	plan    plan
	report  domain.Report
	// withheld is what may not come back, by table. It starts as the deletion journal (§7) and
	// grows: a record that references a withheld object is withheld too, or the restore would
	// write a row pointing at something it deliberately did not create.
	withheld map[string]map[string]bool
	// selected is a SELECTIVE restore's closure, by table. Nil when everything is selected, which
	// is every other mode - a nil map answers "not selected" for everything, so the check has to
	// ask whether the map exists at all rather than what it says.
	selected map[string]map[string]bool
	// remap is the identity a duplicated object was given, by table and old identity.
	remap map[string]string
	// live caches whether the tenant already holds an object, which the DUPLICATE remap asks once
	// per referenced identity rather than once per reference.
	live map[string]bool
	// decided is how many of each entity's records this restore has settled, including the ones it
	// withheld, by the archive's entity name. It is what a resumed attempt skips, and it is
	// written with each batch.
	decided map[string]int
	// passed counts what this attempt has read past, which is how the skipping is done: what has
	// to be the same across attempts is the position in the file.
	passed map[string]int
	// resumeFrom is what an earlier attempt had already decided when it died.
	resumeFrom map[string]int
	// pending is the batch waiting for a transaction, and media the transfers waiting for one to
	// commit.
	pending []staged
	media   []transfer
}

// transfer is one attachment on its way back into the object store: the content address it is
// stored under in the archive, and the key this installation keeps it under.
type transfer struct {
	blob archive.Blob
	key  string
}

// staged is one record on its way into a transaction, with the entity it belongs to.
type staged struct {
	entity archive.Entity
	record archive.Record
}

// prepare reads what the restore has to know before it writes anything: the deletions it may not
// undo, and - for a SELECTIVE restore - what "the collection I named" actually covers.
func (a Applier) prepare(ctx context.Context, p plan) (*state, error) {
	out := &state{
		applier: a, plan: p,
		withheld: map[string]map[string]bool{},
		remap:    map[string]string{},
		live:     map[string]bool{},
		decided:  map[string]int{},
		passed:   map[string]int{},
		// What an earlier attempt got through, and the report it had counted by then. A resumed
		// restore continues both rather than starting either again (BK-7).
		resumeFrom: p.restore.Progress,
		report:     p.restore.Report,
	}
	for entity, count := range p.restore.Progress {
		out.decided[entity] = count
	}

	// §7's measure, and the most effective one because it works without access to old archives:
	// what was deleted between the archive and now does not come back.
	err := a.UnitOfWork.WithinReadOnly(ctx, p.scope, func(ctx context.Context) error {
		return a.Journal.DeletedSince(ctx, p.newest().Manifest.SnapshotAt,
			func(entry repository.Deletion) error {
				out.withhold(entry.Entity, entry.EntityID.String())
				return nil
			})
	})
	if err != nil {
		return nil, err
	}

	if p.restore.Mode == domain.RestoreSelective {
		selected, err := a.selectionOf(ctx, p)
		if err != nil {
			return nil, err
		}
		out.selected = selected
	}
	return out, nil
}

// selectionOf is what a SELECTIVE restore actually covers.
//
// The named containers, everything beneath them, and the named items. The descendants need a pass
// of their own because the archive's order within an entity is by change time rather than by depth
// - a sub-collection can be written before its hub - so a closure computed as the records go past
// would miss whichever children happened to come first. Containers are few and the pass is cheap;
// everything else is decided as it is read, through the reference graph.
func (a Applier) selectionOf(ctx context.Context, p plan) (map[string]map[string]bool, error) {
	parents := map[string]string{}
	entity, known := archive.FindEntityByTable(containerTable)
	if !known {
		return nil, shared.Internalf("backup: no container entity in the archive's list")
	}

	err := a.eachRecord(ctx, p, entity, func(record archive.Record) error {
		if record.Op != archive.OpUpsert {
			return nil
		}
		parent, _ := record.Data["parent_id"].(string)
		parents[record.ID] = parent
		return nil
	})
	if err != nil {
		return nil, err
	}

	selected := map[string]map[string]bool{
		containerTable: {},
		workItemTable:  {},
	}
	for _, id := range p.restore.Selection.ContainerIDs {
		selected[containerTable][id.String()] = true
	}
	for _, id := range p.restore.Selection.ItemIDs {
		selected[workItemTable][id.String()] = true
	}

	// Walk each container up to a named one. Bounded by the number of containers, which is also
	// what stops a cycle - a row whose parent chain does not terminate is one the walk gives up on
	// rather than one it follows for ever.
	for id := range parents {
		at, depth := id, 0
		for at != "" && depth <= len(parents) {
			if selected[containerTable][at] {
				selected[containerTable][id] = true
				break
			}
			at, depth = parents[at], depth+1
		}
	}
	return selected, nil
}

const (
	containerTable = "container"
	workItemTable  = "work_item"
)

// applyEntity reads one entity out of the chain and stages every record it decides to keep.
func (a Applier) applyEntity(ctx context.Context, p plan, out *state, entity archive.Entity) error {
	return a.eachRecord(ctx, p, entity, func(record archive.Record) error {
		return out.stage(ctx, entity, record)
	})
}

// eachRecord hands one entity's records to yield, newest line first.
//
// **Newest first, and each identity only once.** Within one entity a later line supersedes an
// earlier one, so reading the chain from the newest archive backwards and ignoring an identity
// that has already been seen gives the same answer as reading it forwards and overwriting - with
// two differences that matter. A DELETE line seen first keeps the row out, which is what makes an
// incremental chain honest (BK-3, BK-6). And nothing is written twice, so the report counts each
// object once rather than once per archive it appears in.
//
// The set of identities is held only while an entity is being read, and only when the chain has
// more than one archive: a restore from a full archive needs no set at all.
//
// An entity written whole is read from the newest archive alone. Its older copies are superseded
// rather than merged - that is what Whole means, and reading them would restore a row the newest
// archive deliberately no longer has.
func (a Applier) eachRecord(
	ctx context.Context, p plan, entity archive.Entity, yield func(archive.Record) error,
) error {
	sources := p.chain
	if entity.Whole || len(p.chain) == 1 {
		sources = p.chain[:1]
	}

	var seen map[string]bool
	if len(sources) > 1 {
		seen = map[string]bool{}
	}

	for _, description := range sources {
		err := p.reader.Records(ctx, description, entity, p.key, func(record archive.Record) error {
			if seen != nil {
				if seen[record.ID] {
					return nil
				}
				seen[record.ID] = true
			}
			if record.Op == archive.OpDelete {
				// A tombstone is not a row to write. It is the newest word about that identity,
				// and having been seen it now shadows every older line for the same one.
				return nil
			}
			return yield(record)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// withhold marks one object as one that may not come back.
func (s *state) withhold(table, id string) {
	if s.withheld[table] == nil {
		s.withheld[table] = map[string]bool{}
	}
	s.withheld[table][id] = true
}

// stage decides one record and puts it in the batch.
//
// A record an earlier attempt already decided is skipped rather than decided again. The archive is
// immutable and the read order is fixed, so "the first N records of this entity" names the same N
// on every attempt - which is what makes DUPLICATE resumable at all: without it, the second attempt
// would meet the rows the first one wrote and duplicate them (BK-7).
func (s *state) stage(ctx context.Context, entity archive.Entity, record archive.Record) error {
	if s.skip(entity) {
		return nil
	}
	s.decided[entity.Name]++

	// §7 first, before anything else is asked. An object deleted after the archive was taken does
	// not come back, and neither does anything that would point at it.
	if reason, out := s.keptOut(entity, record); out {
		s.withhold(entity.Table, record.ID)
		s.report.Withhold(reason)
		return nil
	}
	if !s.inSelection(entity, record) {
		s.withhold(entity.Table, record.ID)
		s.report.Withhold(domain.WithheldNotSelected)
		return nil
	}
	// A calendar feed is a credential: its token hash is unique across the installation, and a
	// copy of a workspace that duplicated it would make one URL read two workspaces - §8.4's
	// reasoning, and the same unique index would refuse the row anyway. The copy mints its own
	// feeds the way it mints its own tokens: by somebody asking for one.
	if s.plan.remapAll && entity.Table == calendarFeedTable {
		s.withhold(entity.Table, record.ID)
		s.report.Withhold(domain.WithheldExcluded)
		return nil
	}

	s.pending = append(s.pending, staged{entity: entity, record: record})
	if len(s.pending) < s.batch() {
		return nil
	}
	return s.flush(ctx)
}

// skip answers whether this record was already decided by an earlier attempt.
//
// It counts rather than remembering identities: what has to be the same across attempts is the
// position in the file, and a set of identifiers would be a set as large as the archive.
func (s *state) skip(entity archive.Entity) bool {
	s.passed[entity.Name]++
	return s.passed[entity.Name] <= s.resumeFrom[entity.Name]
}

func (s *state) batch() int {
	if s.applier.Batch > 0 {
		return s.applier.Batch
	}
	return DefaultRestoreBatch
}

// keptOut answers whether the deletion journal - or an earlier withholding - keeps this record
// out, and why.
func (s *state) keptOut(entity archive.Entity, record archive.Record) (string, bool) {
	if s.withheld[entity.Table][record.ID] {
		return domain.WithheldDeleted, true
	}
	// A row whose key is made of references - every join table - is named in the journal by
	// neither part, so it is kept out by what it points at. That is also BK-6's second half: an
	// attachment whose medium was erased does not come back either.
	for _, reference := range entity.References {
		id, named := record.Data[reference.Field].(string)
		if named && s.withheld[reference.Table][id] {
			return domain.WithheldDeleted, true
		}
	}
	return "", false
}

// inSelection answers whether a SELECTIVE restore asked for this record.
//
// Either it was named, or it points at something that was. The reference graph does the work: a
// bucket names its collection, an item names its collection, a comment names its item - so
// "everything below the collection I named" falls out of the declarations rather than out of a
// second traversal per entity.
func (s *state) inSelection(entity archive.Entity, record archive.Record) bool {
	if s.selected == nil {
		return true
	}
	if s.selected[entity.Table][record.ID] {
		return true
	}
	for _, reference := range entity.References {
		id, named := record.Data[reference.Field].(string)
		if named && s.selected[reference.Table][id] {
			s.remember(entity.Table, record.ID)
			return true
		}
	}
	return false
}

func (s *state) remember(table, id string) {
	if s.selected[table] == nil {
		s.selected[table] = map[string]bool{}
	}
	s.selected[table][id] = true
}

// flush writes the batch, in one transaction, and then transfers the media it referenced.
//
// The transaction is per batch, which is what §8.3 step 5 asks for: "on cancellation, rollback
// within a transaction per batch size". A restore that is stopped loses the batch in flight and
// keeps everything before it.
//
// The media go afterwards rather than inside. Reading an attachment out of the archive and writing
// it into the object store is two calls to somebody else's machine, and a transaction waiting on
// those holds a pool connection the API shares (observability-reliability.md §8).
func (s *state) flush(ctx context.Context) error {
	if len(s.pending) == 0 {
		return nil
	}
	batch := s.pending
	s.pending = nil

	within := s.applier.UnitOfWork.Within
	if s.plan.dry {
		// A dry run reads and cannot write, and the database is what enforces that rather than a
		// branch: a write that slipped into this path fails loudly instead of quietly succeeding.
		within = s.applier.UnitOfWork.WithinReadOnly
	}

	err := within(ctx, s.plan.scope, func(ctx context.Context) error {
		for _, item := range batch {
			if err := s.write(ctx, item); err != nil {
				return err
			}
		}
		// The marker goes in with the batch, in the same transaction, so that "these records are
		// decided" cannot commit without the records or the other way round. For a NEW_TENANT
		// restore the run row lives in the tenant that asked and the rows land in the one being
		// created, so the two are separate transactions - and it costs nothing, because nothing in
		// a tenant that did not exist a moment ago can collide with what a resumed attempt writes.
		if s.plan.dry {
			return nil
		}
		// The run row lives in the tenant that asked, so "same transaction" is only available
		// when the batch is landing there too - the comparison is against the asker, not against
		// the row's target tenant, which for NEW_TENANT is the same minted identity the batch
		// lands in and exactly the scope the row is invisible from.
		if s.plan.scope.TenantID == s.plan.asker {
			return s.applier.Restores.RecordProgress(ctx, s.plan.restore.ID, s.report, s.decided)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !s.plan.dry && s.plan.scope.TenantID != s.plan.asker {
		err = s.applier.UnitOfWork.Within(ctx, s.plan.asking(), func(ctx context.Context) error {
			return s.applier.Restores.RecordProgress(ctx, s.plan.restore.ID, s.report, s.decided)
		})
		if err != nil {
			return err
		}
	}
	return s.transferMedia(ctx)
}

// write settles one record against the conflict rule and puts it in.
func (s *state) write(ctx context.Context, item staged) error {
	data := maps.Clone(item.record.Data)
	if data == nil {
		data = map[string]any{}
	}

	// The archive's tenant row carries the identity of the tenant it was taken from, and this
	// restore may be going somewhere else. Row level security would refuse the insert either way -
	// the policy on `tenant` compares the row's own id against the current tenant - so rewriting
	// it here is what turns a refusal into a restore.
	if item.entity.Table == tenantTable {
		if s.plan.scope.TenantID.IsZero() {
			return nil
		}
		data["id"] = s.plan.scope.TenantID.String()
		// The slug is unique across the installation, and a copy that kept the source's could
		// never be inserted beside it. A technical one derived from the minted identity - stable
		// across resumed attempts, renameable afterwards - is what a copy deserves.
		if s.plan.remapAll {
			data["slug"] = domain.RestoredSlug(s.plan.scope.TenantID)
		}
	}

	// The NEW_TENANT identity rule (see plan.remapAll): the copy's rows get identities of their
	// own, minted before anything reads data["id"] - the media storage key below is derived from
	// it, and a key minted from the source's identity would collide the way the row would.
	if s.plan.remapAll && item.entity.HasOwnIdentity() && item.entity.Table != tenantTable {
		s.mint(item.entity, data, item.record.ID)
	}

	// A medium's storage key is minted from the tenant it belongs to, and this restore may be
	// going somewhere else. Keeping the archive's key would put two tenants' rows on one object,
	// where deleting either one takes the other's bytes with it.
	storageKey := ""
	if item.entity.Table == mediaObjectTable && !s.plan.scope.TenantID.IsZero() {
		if id, named := data["id"].(string); named {
			mediaID, err := shared.ParseID(id)
			if err != nil {
				return err
			}
			storageKey = media.StorageKeyFor(s.plan.scope.TenantID, mediaID)
			data["storage_key"] = storageKey
		}
	}

	// §8.4's second prohibition, applied where the row is written rather than afterwards: a
	// reminder whose moment passed while the data sat in an archive is marked lapsed, so the
	// scheduler's next pass does not send every one of them at once.
	s.lapse(item.entity, data)

	collided, err := s.applier.Import.Holds(ctx, item.entity.Table, data)
	if err != nil {
		return err
	}

	// The references are remapped whether or not this row collides. A row that is new but points
	// at a container that was duplicated belongs in the duplicate, not beside it.
	if err := s.remapReferences(ctx, item.entity, data); err != nil {
		return err
	}

	rule := ruleOf(s.plan.restore)
	outcome := domain.ConflictSkip
	switch {
	case collided && rule == domain.ConflictOverwrite:
		outcome = domain.ConflictOverwrite
	case collided && rule == domain.ConflictDuplicate && item.entity.Duplicable:
		outcome = domain.ConflictDuplicate
		s.mint(item.entity, data, item.record.ID)
	}

	written := !collided || outcome != domain.ConflictSkip
	if written && !s.plan.dry {
		written, err = s.applier.Import.Write(ctx, item.entity.Table, data,
			outcome == domain.ConflictOverwrite)
		if err != nil {
			return err
		}
	}
	if !written {
		// Nothing went in. Either the rule said to leave the living object alone, or the row's key
		// still belonged to something after the remap - which is the same answer to whoever is
		// reading the report.
		outcome, collided = domain.ConflictSkip, true
	}

	s.report.Count(outcome, collided)
	if written {
		s.report.Contributed(item.entity.Name)
		for _, blob := range item.record.Blobs {
			s.media = append(s.media, transfer{blob: blob, key: storageKey})
		}
	}
	return nil
}

// lapse marks a restored reminder whose moment has gone (backup-restore.md §8.4).
//
// Only a pending one: a reminder that had already been sent, or that somebody cancelled, says
// something about what happened and a restore does not rewrite that. And only one whose moment is
// actually in the past - a restore of last week's archive can carry reminders for next week, and
// those are the ones that should still fire.
func (s *state) lapse(entity archive.Entity, data map[string]any) {
	if entity.Table != reminderTable {
		return
	}
	if state, _ := data["state"].(string); state != string(work.ReminderPending) {
		return
	}
	fireAt, named := data["fire_at"].(string)
	if !named || fireAt == "" {
		// A reminder with no moment yet - a relative one on an item with no due date - has nothing
		// to have missed.
		return
	}
	at, err := time.Parse(time.RFC3339Nano, fireAt)
	if err != nil {
		return
	}
	if at.Before(s.applier.Clock.Now()) {
		data["state"] = string(work.ReminderLapsed)
	}
}

const (
	reminderTable     = "reminder"
	calendarFeedTable = "calendar_feed"
)

// mint gives a duplicated row an identity of its own.
//
// Derived rather than drawn, for the reason domain.DuplicateID gives: a resumed restore has to
// produce the same identifiers, or the half it wrote before it died becomes a second copy nobody
// can tell from the first.
//
// A row whose identity is made of references - every join table - gets no new identity here. Its
// key has already changed, because the rows it joins were remapped, and nothing points at a join
// row for the remap to have to follow.
func (s *state) mint(entity archive.Entity, data map[string]any, originalID string) {
	if !entity.HasOwnIdentity() {
		return
	}
	minted := domain.DuplicateID(s.plan.restore.ID, entity.Name, originalID).String()
	data["id"] = minted
	s.remap[entity.Table+"/"+originalID] = minted
}

// remapReferences points a duplicated object's references at the other duplicates rather than at
// the originals.
//
// The rule is one question per reference: does the tenant already hold what this points at? If it
// does, that object collided too and was - or will be - duplicated under an identity this function
// can derive without having seen it. That is what makes the remap independent of order, which
// matters because the archive's order within an entity is by change time: a parent can be written
// after its child, and a map built as the records went past would miss it.
func (s *state) remapReferences(ctx context.Context, entity archive.Entity, data map[string]any) error {
	if s.plan.remapAll {
		return s.remapEveryReference(entity, data)
	}
	if ruleOf(s.plan.restore) != domain.ConflictDuplicate {
		return nil
	}

	for _, reference := range entity.References {
		id, named := data[reference.Field].(string)
		if !named || id == "" {
			continue
		}
		target, known := archive.FindEntityByTable(reference.Table)
		if !known || !target.Duplicable || !target.HasOwnIdentity() {
			// A reference to something that is never duplicated - an account, a label, a medium -
			// keeps pointing where it pointed. That is the whole reason those are not duplicable:
			// the copy shares the tenant's identities and its attachments rather than doubling
			// them.
			continue
		}
		if minted, mapped := s.remap[reference.Table+"/"+id]; mapped {
			data[reference.Field] = minted
			continue
		}
		live, err := s.holds(ctx, reference.Table, id)
		if err != nil {
			return err
		}
		if !live {
			continue
		}
		minted := domain.DuplicateID(s.plan.restore.ID, target.Name, id).String()
		s.remap[reference.Table+"/"+id] = minted
		data[reference.Field] = minted
	}
	return nil
}

// remapEveryReference is the NEW_TENANT half of the remap: every reference to an entity with an
// identity of its own follows the derivation, unconditionally.
//
// No liveness question and no memory of what was seen, because the answer does not depend on
// either: the copy's identity for a row is a function of the restore, the entity and the original,
// so a reference can be rewritten before or after its target went past and land on the same value.
// That is also what makes a self-reference written child-first - a comment thread, a container
// tree - safe without a second pass.
func (s *state) remapEveryReference(entity archive.Entity, data map[string]any) error {
	for _, reference := range entity.References {
		id, named := data[reference.Field].(string)
		if !named || id == "" {
			continue
		}
		target, known := archive.FindEntityByTable(reference.Table)
		if !known || !target.HasOwnIdentity() || reference.Table == tenantTable {
			continue
		}
		data[reference.Field] = domain.DuplicateID(s.plan.restore.ID, target.Name, id).String()
	}
	return nil
}

// holds asks once per referenced identity rather than once per reference to it.
func (s *state) holds(ctx context.Context, table, id string) (bool, error) {
	key := table + "/" + id
	if answer, asked := s.live[key]; asked {
		return answer, nil
	}
	answer, err := s.applier.Import.Holds(ctx, table, map[string]any{"id": id})
	if err != nil {
		return false, err
	}
	s.live[key] = answer
	return answer, nil
}

// transferMedia puts the attachments the batch referenced back into the object store.
//
// Content-addressed on the way out and keyed on the way in: the archive stores a medium under the
// SHA-256 of its content, and the row says which storage key this installation keeps it under. That
// indirection is what makes an archive restorable somewhere else at all - a storage key carries a
// tenant, a prefix and whatever the bucket layout was that year.
func (s *state) transferMedia(ctx context.Context) error {
	blobs := s.media
	s.media = nil
	if len(blobs) == 0 {
		return nil
	}

	for _, waiting := range blobs {
		s.report.Media++
		if s.plan.dry || waiting.key == "" {
			continue
		}
		if err := s.transferOne(ctx, waiting); err != nil {
			return err
		}
	}
	return nil
}

func (s *state) transferOne(ctx context.Context, waiting transfer) error {
	content, err := s.plan.reader.Medium(ctx, s.plan.chain, waiting.blob.Digest, s.plan.key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The row says the bytes were there and the archive does not have them. The row is
			// restored all the same and the attachment is not: an object whose content is gone is
			// the honest outcome, and refusing the whole restore over one missing file would be
			// the wrong trade on the day this is being used.
			s.report.Withhold(domain.WithheldMediaMissing)
			return nil
		}
		return err
	}
	defer func() { _ = content.Close() }()

	return s.applier.Objects.Put(ctx, storage.Upload{
		Key: waiting.key, Content: content, Size: waiting.blob.Bytes,
		ContentType: mediaContentType,
	})
}

// mediaContentType is what the object store is told an archived attachment is.
//
// The row carries the sniffed type and the store keeps its own; re-sniffing here would mean reading
// the whole attachment twice over somebody else's network to learn what the row already says. The
// generic type is what the port asks for and what the download path replaces from the row.
const mediaContentType = "application/octet-stream"

// ruleOf is the conflict rule a stored restore applies, SKIP unless another was named. The same
// default the request has, read from the row rather than from the request - the row is what the job
// picks up minutes later.
func ruleOf(restore domain.Restore) domain.ConflictRule {
	if restore.ConflictRule == "" {
		return domain.ConflictSkip
	}
	return restore.ConflictRule
}
