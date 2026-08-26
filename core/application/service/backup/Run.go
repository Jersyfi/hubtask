// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	StartBackupName  = "StartBackup"
	GetBackupRunName = "GetBackupRun"
	VerifyBackupName = "VerifyBackup"

	runType = "backup_run"

	// StartedAction is a backup somebody asked for. A warning rather than an info, for the reason
	// creating a target is one: a run is the moment the tenant's data actually leaves, and "who
	// sent a copy of everything to that bucket on Tuesday" is a question with an answer.
	StartedAction audit.Action = "backup.started"
	// VerifiedAction is an archive checked at its target. Info: nothing changes and nothing
	// leaves, and it is recorded because the answer is evidence.
	VerifiedAction audit.Action = "backup.verified"
	// DownloadedAction is fetching an archive, which backup-restore.md §7 calls an auditable data
	// access in its own right. Nothing emits it yet - there is no route that hands an archive to
	// a caller until the restore side of the milestone builds one - and the name is fixed here so
	// that the two halves cannot end up spelling it differently.
	DownloadedAction audit.Action = "backup.downloaded"
)

// Runner is what the three run use cases share.
//
// One struct rather than eight fields repeated three times, for the reason Writer is one: three use
// cases over one aggregate that disagreed about which clock or which queue to use would be three
// chances for a run to be recorded at a moment nothing else agrees with.
type Runner struct {
	Runs       repository.Runs
	Targets    repository.Targets
	Jobs       queue.Queue
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// StartBackup writes one archive to one target, now.
type StartBackup struct{ Runner Runner }

// GetBackupRun answers what one run did.
type GetBackupRun struct{ Runner Runner }

// VerifyBackup checks an archive at its target without restoring it.
type VerifyBackup struct{ Runner Runner }

// StartBackupCommand is the input, typed.
type StartBackupCommand struct {
	TargetID     shared.ID
	Mode         domain.Mode
	IncludeMedia bool
	IncludeAudit bool
}

// Accepted is what a 202 hands back: the job to poll, and the run it will produce.
//
// The run's identifier is also the archive's identifier in the manifest at the target. One name in
// two places rather than a mapping between them.
type Accepted struct {
	JobID shared.ID
	RunID shared.ID
}

// Execute accepts the run and answers the job that will do it.
func (h StartBackup) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd StartBackupCommand,
) (Accepted, error) {
	// The administrator's line rather than the owner's, and the difference is what each act
	// establishes. Creating a target opens a channel the tenant's data may leave by, which is the
	// owner's decision; using a channel that has already been approved is running the workspace.
	if err := h.Runner.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     StartedAction,
		TokenScope: backupManage,
		TargetType: runType,
		TargetID:   cmd.TargetID,
	}); err != nil {
		return Accepted{}, err
	}

	mode := cmd.Mode
	if mode == "" {
		// FULL by default, and deliberately: a run somebody asks for by hand is usually asked for
		// because something is about to happen, and an incremental that turns out to have no
		// parent at the target is the wrong thing to find out then.
		mode = domain.ModeFull
	}
	if !mode.Valid() {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeScheduleModeInvalid).
			WithFields(shared.FieldError{Path: "/mode", Code: domain.CodeScheduleModeInvalid})
	}

	runID := h.Runner.IDs.NewID()
	now := h.Runner.Clock.Now()
	var accepted Accepted

	err := h.Runner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		target, err := h.Runner.Targets.Find(ctx, cmd.TargetID)
		if err != nil {
			return err
		}
		if !target.Enabled {
			return shared.ErrConflict.WithDetail(domain.CodeTargetDisabled).
				WithParams(map[string]string{"target_id": target.ID.String()})
		}

		parentID, err := h.Runner.parentFor(ctx, mode, cmd.TargetID)
		if err != nil {
			return err
		}

		jobID, err := h.Runner.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindBackupRun,
			TenantID: actor.TenantID,
			// The target, so that two requests to back up the same target collapse into the one
			// that is already happening. It is the lock §5 asks for, expressed in the queue as
			// well as in the table - the queue stops the second job being created, and the table
			// stops a second run claiming the target if one is created anyway.
			DedupeKey: string(queue.KindBackupRun) + ":" + cmd.TargetID.String(),
			Payload: map[string]any{
				"run_id":        runID.String(),
				"target_id":     cmd.TargetID.String(),
				"mode":          string(mode),
				"parent_run_id": parentID.String(),
				"include_media": cmd.IncludeMedia,
				"include_audit": cmd.IncludeAudit,
				"trigger":       string(domain.TriggerManual),
			},
		})
		if err != nil {
			return err
		}
		accepted = Accepted{JobID: jobID, RunID: runID}

		return h.Runner.record(ctx, actor, StartedAction, audit.SeverityWarning, runID, now,
			[]audit.Change{
				{Field: "target_id", Classification: audit.Open, To: cmd.TargetID.String()},
				{Field: "mode", Classification: audit.Open, To: string(mode)},
			})
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// parentFor answers the archive an incremental continues, and refuses an incremental that has none.
//
// A refusal rather than a quiet promotion to a full run. A caller that asked for an incremental and
// silently got a full one has been told nothing about the thing it actually cares about: how long
// the run will take and how much it will transfer.
func (r Runner) parentFor(
	ctx context.Context, mode domain.Mode, targetID shared.ID,
) (shared.ID, error) {
	if mode != domain.ModeIncremental {
		return "", nil
	}
	parent, err := r.Runs.LatestSuccessful(ctx, targetID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return "", shared.ErrConflict.WithDetail(domain.CodeNoParentArchive).
				WithParams(map[string]string{"target_id": targetID.String()})
		}
		return "", err
	}
	return parent.ID, nil
}

// Execute answers one run.
func (h GetBackupRun) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Run, error) {
	if err := h.Runner.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     StartedAction,
		TokenScope: backupRead,
		TargetType: runType,
		TargetID:   id,
	}); err != nil {
		return domain.Run{}, err
	}

	var run domain.Run
	err := h.Runner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		run, err = h.Runner.Runs.Find(ctx, id)
		return err
	})
	if err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

// Execute accepts the verification and answers the job that will do it.
func (h VerifyBackup) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (Accepted, error) {
	if err := h.Runner.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     VerifiedAction,
		TokenScope: backupManage,
		TargetType: runType,
		TargetID:   id,
	}); err != nil {
		return Accepted{}, err
	}

	now := h.Runner.Clock.Now()
	var accepted Accepted

	err := h.Runner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		run, err := h.Runner.Runs.Find(ctx, id)
		if err != nil {
			return err
		}
		// There is nothing at the target to check for a run that never finished writing one, and
		// a verification that answered "no" for that reason would read as a corrupt archive.
		if !run.Succeeded() || run.ArchivePath == "" {
			return shared.ErrConflict.WithDetail(domain.CodeRunHasNoArchive).
				WithParams(map[string]string{"run_id": id.String(), "status": string(run.Status)})
		}

		jobID, err := h.Runner.Jobs.Enqueue(ctx, queue.Request{
			Kind:      queue.KindBackupVerify,
			TenantID:  actor.TenantID,
			DedupeKey: string(queue.KindBackupVerify) + ":" + id.String(),
			Payload: map[string]any{
				"run_id":    id.String(),
				"target_id": run.TargetID.String(),
			},
		})
		if err != nil {
			return err
		}
		accepted = Accepted{JobID: jobID, RunID: id}

		return h.Runner.record(ctx, actor, VerifiedAction, audit.SeverityInfo, id, now,
			[]audit.Change{
				{Field: "target_id", Classification: audit.Open, To: run.TargetID.String()},
			})
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// record writes the audit entry. Identifiers and codes only - never an archive path, which carries
// a tenant identifier and a timestamp and belongs in the row rather than in the trail.
func (r Runner) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	severity audit.Severity, targetID shared.ID, at time.Time, changes []audit.Change,
) error {
	return r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: at,
		Action: action, Outcome: audit.OutcomeSuccess, Severity: severity,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: runType, TargetID: targetID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// Descriptor registers starting a backup in all three channels.
func (h StartBackup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartBackupName,
		Summary: "Writes one archive to one backup target, now, rather than waiting for a " +
			"schedule. Answers the job that will do it and the run it will produce - and the " +
			"run's identifier is the archive's identifier in the manifest at the target, so a " +
			"caller that has one has the other. A second request for a target that is already " +
			"being backed up answers the run that is already happening.",
		SideEffects: "Enqueues a backup job and writes an audit entry. The archive itself is " +
			"written by the job, minutes later.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "Where to write the archive.",
			},
			{
				Name: "mode", Kind: usecase.KindString,
				Enum: []string{"FULL", "INCREMENTAL"},
				Description: "FULL unless said otherwise, because a run somebody asks for by " +
					"hand is usually asked for because something is about to happen. An " +
					"INCREMENTAL with no earlier archive at the target is refused rather than " +
					"quietly promoted.",
			},
			{
				Name: "include_media", Kind: usecase.KindBool,
				Description: "Whether attachments travel with the archive.",
			},
			{
				Name: "include_audit", Kind: usecase.KindBool,
				Description: "Whether the audit trail travels with it. Better evidence, and " +
					"personal metadata kept for longer.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: StartedAction, TargetType: runType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartBackup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	var mode domain.Mode
	if named := in.OptionalString("mode"); named != nil {
		mode = domain.Mode(*named)
	}

	accepted, err := h.Execute(ctx, actor, StartBackupCommand{
		TargetID: targetID, Mode: mode,
		// Both default to true, because a backup that quietly left the attachments behind is not
		// the backup anybody meant. Absent and false have to be told apart for that, which is
		// what Present is for.
		IncludeMedia: !in.Present("include_media") || in.Bool("include_media"),
		IncludeAudit: !in.Present("include_audit") || in.Bool("include_audit"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{"job_id": accepted.JobID.String(), "run_id": accepted.RunID.String()}, nil
}

// Descriptor registers the run resource.
func (h GetBackupRun) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetBackupRunName,
		Summary: "What one backup run did: what was written, where, how much of it, when the " +
			"snapshot it represents was taken, and whether it has been verified since. Read from " +
			"the database rather than from the target - the listing at the target is the reading " +
			"that survives losing the database, and this is the one to poll while a run is still " +
			"going.",
		SideEffects: "None. Reads only.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "run_id", Kind: usecase.KindID, Required: true, Description: "Which run."},
		},
		Audit: usecase.AuditDeclaration{
			Action: StartedAction, TargetType: runType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetBackupRun) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("run_id")
	if err != nil {
		return nil, err
	}
	run, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return runOutput(run), nil
}

// Descriptor registers the verification.
func (h VerifyBackup) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: VerifyBackupName,
		Summary: "Checks an archive at its target without restoring it: every member is read and " +
			"its checksum compared, and the manifest and checksums.txt have to agree with each " +
			"other. It needs no archive key - the checksums are over the bytes as stored. The " +
			"answer is written onto the run as verified_at and verify_ok.",
		SideEffects: "Enqueues a verification job and writes an audit entry. Nothing at the " +
			"target is changed.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "run_id", Kind: usecase.KindID, Required: true,
				Description: "The run whose archive to check.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: VerifiedAction, TargetType: runType,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h VerifyBackup) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("run_id")
	if err != nil {
		return nil, err
	}
	accepted, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"job_id": accepted.JobID.String(), "run_id": accepted.RunID.String()}, nil
}

// runOutput is a run as the three channels answer it.
func runOutput(run domain.Run) usecase.Output {
	out := usecase.Output{
		"id":         run.ID.String(),
		"target_id":  run.TargetID.String(),
		"trigger":    string(run.Trigger),
		"mode":       string(run.Mode),
		"status":     string(run.Status),
		"started_at": run.StartedAt,
	}
	for name, id := range map[string]shared.ID{
		"schedule_id": run.ScheduleID, "parent_run_id": run.ParentRunID,
	} {
		if !id.IsZero() {
			out[name] = id.String()
		}
	}
	if run.ArchivePath != "" {
		out["archive_path"] = run.ArchivePath
	}
	for name, value := range map[string]int64{
		"size_bytes": run.SizeBytes, "item_count": int64(run.ItemCount),
		"media_count": int64(run.MediaCount),
	} {
		if value != 0 {
			out[name] = value
		}
	}
	for name, at := range map[string]time.Time{
		"snapshot_at": run.SnapshotAt, "finished_at": run.FinishedAt,
		"expires_at": run.ExpiresAt, "verified_at": run.VerifiedAt,
	} {
		if !at.IsZero() {
			out[name] = at
		}
	}
	if run.ErrorCode != "" {
		out["error_code"] = run.ErrorCode
	}
	if run.VerifyOK != nil {
		out["verify_ok"] = *run.VerifyOK
	}
	return out
}
