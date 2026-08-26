// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListBackupsAtTargetName = "ListBackupsAtTarget"
	StartRestoreName        = "StartRestore"
	GetRestoreRunName       = "GetRestoreRun"

	restoreType = "restore_run"

	// StartedRestoreAction is a restore somebody asked for. A warning: it is the one path in this
	// milestone that changes a tenant's data from outside the tenant's own use cases, and "who
	// restored what on Tuesday" is a question with an answer.
	StartedRestoreAction audit.Action = "backup.restore_started"
	// FinishedRestoreAction is what the restore did, written when it is over. §8.3 step 6 asks for
	// the report to reach the audit, and this is where it arrives - counts, never content.
	FinishedRestoreAction audit.Action = "backup.restore_finished"
)

// Restorer is what the restore-side use cases share.
//
// One struct for the same reason Writer and Runner are one each: three use cases over one act that
// disagreed about which cipher or which clock to use would be three chances for an archive to be
// listed under one set of assumptions and restored under another.
type Restorer struct {
	Targets    repository.Targets
	Restores   repository.Restores
	Workspace  repository.Workspace
	Jobs       queue.Queue
	StepUp     stepup.Verifier
	Encryptor  crypto.Encryptor
	Opener     backupstorage.Opener
	Cipher     crypto.StreamCipher
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// ListBackupsAtTarget answers what is at a target, read from the manifests there.
type ListBackupsAtTarget struct{ Restorer Restorer }

// StartRestore accepts a restore and answers the job that will do it.
type StartRestore struct{ Restorer Restorer }

// GetRestoreRun answers what one restore did, or is about to do.
type GetRestoreRun struct{ Restorer Restorer }

// Archive is one archive as the listing answers it.
//
// It is assembled from the target alone. No row in any database is consulted, which is the
// property §8.1 promises in as many words - "a restore works even when the database is lost and
// only the target credentials exist" - and the reason `backup_run` calls itself "a log and an
// accelerator, not a prerequisite for a restore".
type Archive struct {
	ArchiveID string
	// Path is the archive's directory at the target, and it is what a restore is asked for. A
	// path rather than a run identifier, because a run identifier only means something to a
	// database.
	Path            string
	CreatedAt       time.Time
	Mode            string
	ParentArchiveID string
	ScopeKind       string
	ScopeID         string
	SizeBytes       int64
	ItemCount       int64
	MediaCount      int64
	SchemaVersion   string
	ProductVersion  string
	Encrypted       bool
	EncryptionKeyID string
	// Complete is whether the run that wrote it finished. An archive without `checksums.txt` is
	// not damaged; it is a run still going or one that died, and whoever is choosing what to
	// restore from has to be able to tell those apart.
	Complete bool
}

// Execute lists the archives one target holds for this tenant.
//
// The one database read is the target's own row and its credential, which is what opening a
// connection needs and nothing more. Everything the answer is made of comes from manifests at the
// target - so the day this matters, when the database is a fresh empty one and all that is left is
// a bucket and a key, the listing still works.
func (h ListBackupsAtTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, targetID, scopeID shared.ID,
) ([]Archive, error) {
	if targetID.IsZero() {
		return nil, shared.ErrValidation.WithDetail("backup.target_id_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "backup.target_id_required"})
	}
	// BK-10 begins here, before anything is read: a caller asking for another tenant's archives is
	// refused rather than answered with an empty list. An empty list would be a wrong answer to a
	// question nobody may ask.
	if !scopeID.IsZero() && scopeID != actor.TenantID {
		return nil, shared.ErrValidation.WithDetail(domain.CodeRestoreArchiveScopeMismatch).
			WithFields(shared.FieldError{
				Path: "/tenant_id", Code: domain.CodeRestoreArchiveScopeMismatch,
			})
	}
	if err := h.Restorer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetChangedAction,
		TokenScope: backupRead,
		TargetType: targetType,
		TargetID:   targetID,
	}); err != nil {
		return nil, err
	}

	store, err := h.Restorer.open(ctx, actor.PersistenceScope(), targetID)
	if err != nil {
		return nil, err
	}

	// Outside a transaction. A listing is a call to somebody else's machine and can take as long
	// as they feel like taking (observability-reliability.md §8), and there is nothing in the
	// database this answer needs to be consistent with.
	described, err := archive.NewReader(store, h.Restorer.Cipher).List(ctx, "")
	if err != nil {
		return nil, err
	}

	// The tenant's own archives and nobody else's. The name is the filter, because a target can be
	// shared and the storage port's prefix is a place rather than a string: an archive's name is a
	// directory under the target's root. BK-10's listing half.
	mine := archive.Prefix(actor.TenantID)
	archives := make([]Archive, 0, len(described))
	for _, description := range described {
		if !strings.HasPrefix(description.Prefix, mine) {
			continue
		}
		archives = append(archives, archiveOf(description))
	}
	return archives, nil
}

// archiveOf is one description as the listing answers it. Manifest fields only, and no user
// content among them - the manifest is the one member of an archive that is never encrypted, and
// whoever holds the storage can read it (rule 10).
func archiveOf(description archive.Description) Archive {
	manifest := description.Manifest
	var items int64
	for _, count := range manifest.Counts {
		items += count
	}
	return Archive{
		ArchiveID:       manifest.ArchiveID,
		Path:            description.Prefix,
		CreatedAt:       manifest.SnapshotAt,
		Mode:            string(manifest.Mode),
		ParentArchiveID: manifest.ParentID,
		ScopeKind:       string(manifest.Scope.Kind),
		ScopeID:         manifest.Scope.ID,
		SizeBytes:       description.Bytes,
		ItemCount:       items,
		MediaCount:      manifest.MediaCount,
		SchemaVersion:   manifest.SchemaVersion,
		ProductVersion:  manifest.ProductVersion,
		Encrypted:       manifest.Encryption.IsEncrypted(),
		EncryptionKeyID: manifest.Encryption.KeyID,
		Complete:        description.Complete,
	}
}

// open reads one target and hands back something to talk to it with.
//
// The transaction is as short as it can be - a row and a sealed credential - and the connection is
// used outside it. A target is somebody else's machine, and a transaction waiting on one holds a
// pool connection the API shares (observability-reliability.md §8).
func (r Restorer) open(
	ctx context.Context, scope persistence.Scope, targetID shared.ID,
) (backupstorage.Store, error) {
	var store backupstorage.Store
	err := r.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		target, err := r.Targets.Find(ctx, targetID)
		if err != nil {
			return err
		}
		sealed, err := r.Targets.Credential(ctx, targetID)
		if err != nil {
			return err
		}
		credentials, err := unsealCredentials(ctx, r.Encryptor, targetID, sealed)
		if err != nil {
			return err
		}
		store, err = r.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

// Descriptor registers the listing.
func (h ListBackupsAtTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListBackupsAtTargetName,
		Summary: "Lists the archives one backup target holds for this tenant, read from the " +
			"manifests at the target rather than from the database. That is what makes it the " +
			"reading that survives a total loss: with the target's credentials and nothing else, " +
			"this still answers. Each entry says when it was taken, what it covers, how large it " +
			"is, whether it is full or incremental, which archive it continues, which key it is " +
			"encrypted under, and whether the run that wrote it finished.",
		SideEffects: "None. Reads at the target and writes nothing anywhere.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "Which target to read.",
			},
			{
				Name: "tenant_id", Kind: usecase.KindID,
				Description: "Whose archives. Only your own, and asking for another's is " +
					"refused rather than answered empty.",
			},
			{
				Name: "refresh", Kind: usecase.KindBool,
				Description: "Read the target again rather than answering from a cache. This " +
					"build has no cache, so it reads again either way.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TargetChangedAction, TargetType: targetType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListBackupsAtTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	var scopeID shared.ID
	if named := in.OptionalString("tenant_id"); named != nil && *named != "" {
		scopeID, err = shared.ParseID(*named)
		if err != nil {
			return nil, err
		}
	}

	archives, err := h.Execute(ctx, actor, targetID, scopeID)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(archives))
	for _, found := range archives {
		rows = append(rows, archiveOutput(found))
	}
	return usecase.Output{"data": rows}, nil
}

// archiveOutput is one archive as the three channels answer it.
func archiveOutput(found Archive) usecase.Output {
	out := usecase.Output{
		"archive_id":      found.ArchiveID,
		"path":            found.Path,
		"created_at":      found.CreatedAt,
		"mode":            found.Mode,
		"size_bytes":      found.SizeBytes,
		"item_count":      found.ItemCount,
		"media_count":     found.MediaCount,
		"schema_version":  found.SchemaVersion,
		"product_version": found.ProductVersion,
		"encrypted":       found.Encrypted,
		"complete":        found.Complete,
		"scope": map[string]any{
			"kind": found.ScopeKind,
			"id":   found.ScopeID,
		},
	}
	for name, value := range map[string]string{
		"parent_archive_id": found.ParentArchiveID,
		"encryption_key_id": found.EncryptionKeyID,
	} {
		if value != "" {
			out[name] = value
		}
	}
	return out
}

// Execute accepts a restore and answers the job that will do it.
//
// Everything §8.3 puts in front of a destructive mode is asked here rather than in the job: a
// refusal a caller can read is worth more than one that arrives minutes later in a run's error
// code, and the confirmation is a thing a person typed - it belongs to the request that carried
// it.
func (h StartRestore) Execute(
	ctx context.Context, actor appshared.ActorContext, request domain.RestoreRequest,
) (Accepted, error) {
	if err := request.Validate(); err != nil {
		return Accepted{}, err
	}
	if err := h.authorise(ctx, actor, request); err != nil {
		return Accepted{}, err
	}

	// The tenant being restored into. A mode that writes into a living tenant has to be this one -
	// asking for another's is the cross-tenant restore BK-10 forbids, and it is refused here
	// rather than found out at the archive's manifest.
	if !request.TenantID.IsZero() && request.TenantID != actor.TenantID {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeRestoreArchiveScopeMismatch).
			WithFields(shared.FieldError{
				Path: "/target_tenant_id", Code: domain.CodeRestoreArchiveScopeMismatch,
			})
	}

	restoreID := h.Restorer.IDs.NewID()
	now := h.Restorer.Clock.Now()
	into := request.TenantID
	if request.Mode == domain.RestoreNewTenant {
		// The identifier is minted here rather than taken from the request, which is what makes
		// the job's elevation into it safe: there is nothing of anybody else's under a tenant that
		// did not exist a moment ago.
		into = h.Restorer.IDs.NewID()
	}

	var accepted Accepted
	err := h.Restorer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.confirm(ctx, actor, request); err != nil {
			return err
		}
		// §8.3 has one restore at a time, and the refusal belongs where the caller can read it
		// rather than minutes later inside a job.
		running, err := h.Restorer.Restores.InProgress(ctx)
		if err != nil {
			return err
		}
		if running {
			return shared.ErrConflict.WithDetail(domain.CodeRestoreTargetBusy)
		}
		if _, err := h.Restorer.Targets.Find(ctx, request.TargetID); err != nil {
			return err
		}

		err = h.Restorer.Restores.Insert(ctx, domain.Restore{
			ID: restoreID, TargetID: request.TargetID, TenantID: into,
			SourceArchive: request.SourceArchive, Mode: request.Mode,
			ConflictRule: request.RuleOrDefault(), Selection: request.Selection,
			DryRun:             request.DryRun || !request.Mode.Writes(),
			CreateSafetyBackup: request.CreateSafetyBackup,
			Status:             domain.RestorePending, RequestedBy: actor.AccountID,
		})
		if err != nil {
			return err
		}

		jobID, err := h.Restorer.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindBackupRestore,
			TenantID: actor.TenantID,
			// The restore rather than the tenant: two jobs for one restore are the same work, and
			// two restores in one tenant have already been refused above.
			DedupeKey: string(queue.KindBackupRestore) + ":" + restoreID.String(),
			Payload: map[string]any{
				"restore_id": restoreID.String(),
			},
		})
		if err != nil {
			return err
		}
		accepted = Accepted{JobID: jobID, RunID: restoreID}

		return h.Restorer.record(ctx, actor, StartedRestoreAction, audit.SeverityWarning,
			restoreID, now, []audit.Change{
				{Field: "target_id", Classification: audit.Open, To: request.TargetID.String()},
				{Field: "mode", Classification: audit.Open, To: string(request.Mode)},
				{Field: "dry_run", Classification: audit.Open,
					To: map[bool]string{true: "true", false: "false"}[request.DryRun]},
			})
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// authorise asks for the right the mode actually needs.
//
// Two lines, and the split is the same one a backup target draws. Reading an archive back into the
// workspace is running the workspace - the administrator's line. Replacing a tenant with an archive
// is the one thing an administrator cannot do (domain-model.md §3.2): it destroys what is there,
// and the matrix's line for destroying is the owner's.
func (h StartRestore) authorise(
	ctx context.Context, actor appshared.ActorContext, request domain.RestoreRequest,
) error {
	permission := service.PermissionStructure
	if request.Mode.Destructive() {
		permission = service.PermissionDeleteContainer
	}
	return h.Restorer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     StartedRestoreAction,
		TokenScope: backupManage,
		TargetType: restoreType,
		TargetID:   request.TargetID,
	})
}

// confirm is §8.3 step 3: the tenant's name typed, and a step-up on top of it.
//
// Both are asked only of the destructive modes, and both are refusals rather than warnings. The
// step-up is the one this installation cannot yet satisfy, and it says so in its own code: "you did
// not prove it" and "nothing here can prove it" are different problems, and an operator who reads
// the second knows to import the archive as a new tenant instead.
func (h StartRestore) confirm(
	ctx context.Context, actor appshared.ActorContext, request domain.RestoreRequest,
) error {
	if !request.Mode.Destructive() {
		return nil
	}

	name, err := h.Restorer.Workspace.Name(ctx)
	if err != nil {
		return err
	}
	if !request.ConfirmationMatches(name) {
		return shared.ErrValidation.WithDetail(domain.CodeRestoreConfirmationRequired).
			WithParams(map[string]string{"name": name}).
			WithFields(shared.FieldError{
				Path: "/confirmation", Code: domain.CodeRestoreConfirmationRequired,
			})
	}

	if h.Restorer.StepUp == nil || !h.Restorer.StepUp.Available() {
		return shared.ErrConflict.WithDetail(domain.CodeStepUpUnavailable)
	}
	satisfied, err := h.Restorer.StepUp.Satisfied(ctx, actor.AccountID, request.StepUpToken)
	if err != nil {
		return err
	}
	if !satisfied {
		return shared.ErrValidation.WithDetail(domain.CodeStepUpRequired).
			WithFields(shared.FieldError{Path: "/step_up_token", Code: domain.CodeStepUpRequired})
	}
	return nil
}

// Execute answers one restore.
func (h GetRestoreRun) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Restore, error) {
	if err := h.Restorer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     StartedRestoreAction,
		TokenScope: backupRead,
		TargetType: restoreType,
		TargetID:   id,
	}); err != nil {
		return domain.Restore{}, err
	}

	var restore domain.Restore
	err := h.Restorer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		restore, err = h.Restorer.Restores.Find(ctx, id)
		return err
	})
	if err != nil {
		return domain.Restore{}, err
	}
	return restore, nil
}

// record writes an audit entry about a restore. Identifiers and codes only - never an archive path,
// which carries a tenant identifier and a timestamp and belongs in the row rather than in the trail
// (rules 8 and 10).
func (r Restorer) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	severity audit.Severity, targetID shared.ID, at time.Time, changes []audit.Change,
) error {
	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at,
		Action: action, Outcome: audit.OutcomeSuccess, Severity: severity,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: restoreType, TargetID: targetID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// Descriptor registers starting a restore in all three channels.
func (h StartRestore) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartRestoreName,
		Summary: "Reads an archive back. A dry run by default: it reports what it would do - how " +
			"many objects are new, overwritten, skipped and in conflict - and changes nothing. " +
			"Six modes: INSPECT looks, SELECTIVE pulls named collections or items back, MERGE " +
			"imports and settles each collision by rule, REPLACE_TENANT resets the workspace to " +
			"the archive, NEW_TENANT imports it alongside as a workspace of its own, and " +
			"INSTANCE restores a system backup. NEW_TENANT is the way to check before a " +
			"destructive mode: import beside, look, then decide. No automation fires, no webhook " +
			"is sent, no reminder is caught up, and no token or session is restored - people sign " +
			"in again and personal access tokens are recreated.",
		SideEffects: "Enqueues a restore job and writes an audit entry. The rows themselves are " +
			"written by the job, and a destructive mode takes a backup of the current state first.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "Which target holds the archive.",
			},
			{
				Name: "archive_id", Kind: usecase.KindString, Required: true,
				Description: "Which archive - the path the listing at the target gave you. A " +
					"path rather than a run identifier, because a restore has to work when the " +
					"database that recorded the run is gone.",
			},
			{
				Name: "mode", Kind: usecase.KindString, Required: true,
				Enum:        []string{"INSPECT", "SELECTIVE", "MERGE", "REPLACE_TENANT", "NEW_TENANT", "INSTANCE"},
				Description: "What to do with what is read.",
			},
			{
				Name: "target_tenant_id", Kind: usecase.KindID,
				Description: "Which workspace to restore into. Your own, and leave it out for " +
					"NEW_TENANT and INSTANCE, which do not go into one that already exists.",
			},
			{
				Name: "conflict_rule", Kind: usecase.KindString,
				Enum: []string{"SKIP", "OVERWRITE", "DUPLICATE"},
				Description: "How to settle an object that is in the archive and in the " +
					"workspace. SKIP unless said otherwise, because the living object is the one " +
					"somebody has been working in. DUPLICATE imports the archive's version " +
					"beside it under a new identity; it does not apply to accounts, labels or " +
					"attachments, which are shared rather than copied.",
			},
			{
				Name: "dry_run", Kind: usecase.KindBool,
				Description: "Report what would happen and change nothing. True unless said " +
					"otherwise.",
			},
			{
				Name: "create_safety_backup", Kind: usecase.KindBool,
				Description: "Take a backup of the current state before a destructive mode. " +
					"True unless said otherwise, and a destructive mode with nowhere to write " +
					"one is refused rather than carried out.",
			},
			{
				Name: "confirmation", Kind: usecase.KindString,
				Description: "For a destructive mode, the workspace's name, typed exactly.",
			},
			{
				Name: "selection_container_ids", Kind: usecase.KindIDList,
				Description: "For a SELECTIVE restore, the collections and hubs to bring back - " +
					"and with them everything that hangs off one.",
			},
			{
				Name: "selection_item_ids", Kind: usecase.KindIDList,
				Description: "For a SELECTIVE restore, the entries to bring back.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "For a destructive mode, the proof of a fresh, stronger " +
					"authentication. No installation can issue one yet, so a destructive mode is " +
					"refused rather than silently permitted.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: StartedRestoreAction, TargetType: restoreType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartRestore) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	request := domain.RestoreRequest{
		TargetID:      targetID,
		SourceArchive: in.String("archive_id"),
		Mode:          domain.RestoreMode(in.String("mode")),
		// Both default to true, because the safe reading of a partly specified request has to be
		// the safe one: a restore nobody said was real is a rehearsal, and a destructive one
		// nobody said to skip the copy for gets the copy.
		DryRun:             !in.Present("dry_run") || in.Bool("dry_run"),
		CreateSafetyBackup: !in.Present("create_safety_backup") || in.Bool("create_safety_backup"),
		Confirmation:       in.String("confirmation"),
		StepUpToken:        in.String("step_up_token"),
	}
	if rule := in.String("conflict_rule"); rule != "" {
		request.ConflictRule = domain.ConflictRule(rule)
	}
	if named := in.OptionalString("target_tenant_id"); named != nil && *named != "" {
		request.TenantID, err = shared.ParseID(*named)
		if err != nil {
			return nil, err
		}
	}
	if request.Selection, err = selectionIn(in); err != nil {
		return nil, err
	}

	accepted, err := h.Execute(ctx, actor, request)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"job_id": accepted.JobID.String(), "restore_id": accepted.RunID.String(),
	}, nil
}

// selectionIn reads what a SELECTIVE restore named.
func selectionIn(in usecase.Input) (domain.Selection, error) {
	var selection domain.Selection
	for name, into := range map[string]*[]shared.ID{
		"selection_container_ids": &selection.ContainerIDs,
		"selection_item_ids":      &selection.ItemIDs,
	} {
		if !in.Present(name) {
			continue
		}
		ids, err := in.IDList(name)
		if err != nil {
			return domain.Selection{}, err
		}
		*into = ids
	}
	return selection, nil
}

// Descriptor registers the restore resource.
func (h GetRestoreRun) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetRestoreRunName,
		Summary: "What one restore did, or is about to do. It carries the report of the dry run - " +
			"new, overwritten, skipped, duplicated, in conflict, and what was deliberately " +
			"withheld - which is what to read before asking for the same restore without " +
			"dry_run. It also names the backup taken before a destructive mode, so the way back " +
			"is an identifier rather than a search at the target.",
		SideEffects: "None. Reads only.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "restore_id", Kind: usecase.KindID, Required: true, Description: "Which restore."},
		},
		Audit: usecase.AuditDeclaration{
			Action: StartedRestoreAction, TargetType: restoreType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetRestoreRun) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("restore_id")
	if err != nil {
		return nil, err
	}
	restore, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return restoreOutput(restore), nil
}

// restoreOutput is one restore as the three channels answer it.
func restoreOutput(restore domain.Restore) usecase.Output {
	out := usecase.Output{
		"id":             restore.ID.String(),
		"target_id":      restore.TargetID.String(),
		"source_archive": restore.SourceArchive,
		"mode":           string(restore.Mode),
		"conflict_rule":  string(restore.ConflictRule),
		"dry_run":        restore.DryRun,
		"status":         string(restore.Status),
		"report":         reportOutput(restore.Report),
	}
	for name, id := range map[string]shared.ID{
		"tenant_id": restore.TenantID, "safety_backup_run_id": restore.SafetyRunID,
	} {
		if !id.IsZero() {
			out[name] = id.String()
		}
	}
	for name, at := range map[string]time.Time{
		"started_at": restore.StartedAt, "finished_at": restore.FinishedAt,
	} {
		if !at.IsZero() {
			out[name] = at
		}
	}
	if restore.ErrorCode != "" {
		out["error_code"] = restore.ErrorCode
	}
	return out
}

// reportOutput is the report as the contract spells it. `deleted` is a reading of `withheld` rather
// than a counter of its own: two numbers that have to agree are two numbers that eventually do not.
func reportOutput(report domain.Report) map[string]any {
	out := map[string]any{
		"new":         report.New,
		"overwritten": report.Overwritten,
		"skipped":     report.Skipped,
		"duplicated":  report.Duplicated,
		"conflicts":   report.Conflicts,
		"deleted":     report.Deleted(),
		"media":       report.Media,
	}
	if len(report.Withheld) > 0 {
		out["withheld"] = report.Withheld
	}
	if len(report.Entities) > 0 {
		out["entities"] = report.Entities
	}
	return out
}
