// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The trial restore (B-4, P-14, backup-restore.md §5): the run that wrote a FULL archive reads it
// back, in the same job, as an INSPECT restore - every member read, every checksum verified,
// every encrypted member decrypted with the key the schedule names, and the difference report
// against the workspace produced and kept on the run. An archive the product cannot read back is
// not a backup, so a trial that fails fails the run.

// CodeTrialRestoreFailed is the run's error code when its own archive could not be read back.
const CodeTrialRestoreFailed = "backup.trial_restore_failed"

// InspectInput is one trial: the archive just written, at the store that holds it.
type InspectInput struct {
	TenantID shared.ID
	TargetID shared.ID
	Store    backupstorage.Store
	Archive  string
	Report   func(float64)
}

// Inspector reads an archive back and says what a restore of it would do. The Applier is the one
// implementation; an interface so that the performer's tests can stand one in.
type Inspector interface {
	Inspect(ctx context.Context, in InspectInput) (domain.Report, error)
}

// Trial is what the run keeps of its trial: when, the report where it succeeded, and where it
// failed the code and the member that failed.
type Trial struct {
	InspectedAt time.Time       `json:"inspected_at"`
	Report      *map[string]any `json:"report,omitempty"`
	Failure     *TrialFailure   `json:"failure,omitempty"`
}

// TrialFailure names what could not be read back: the reader's code, and the member where it
// stopped - a path at the target, never content.
type TrialFailure struct {
	Code   string `json:"code"`
	Member string `json:"member,omitempty"`
}

// Inspect is the trial's whole work: the archive's checksums verified member by member, then the
// dry apply that decrypts and reads every entity and compares it with the workspace. Nothing is
// written; a dry run opens read-only transactions, and the database enforces that rather than a
// branch.
func (a Applier) Inspect(ctx context.Context, in InspectInput) (domain.Report, error) {
	reader := archive.NewReader(in.Store, a.Cipher)
	if err := reader.Verify(ctx, in.Archive); err != nil {
		return domain.Report{}, err
	}
	restore := domain.Restore{
		TargetID: in.TargetID, TenantID: in.TenantID, SourceArchive: in.Archive,
		Mode: domain.RestoreInspect, DryRun: true,
	}
	chain, key, err := a.precheck(ctx, reader, restore, in.TenantID)
	if err != nil {
		return domain.Report{}, err
	}
	scope := persistence.Scope{TenantID: in.TenantID}
	return a.apply(ctx, plan{
		restore: restore, chain: chain, key: key, reader: reader,
		scope: scope, asker: in.TenantID, report: in.Report, dry: true,
	})
}

// trial reads the archive a run just wrote back through the inspector and answers what to keep on
// the run. A failure is answered as the trial's record and as the error that fails the run.
func (p Performer) trial(ctx context.Context, in PerformInput, store backupstorage.Store, prefix string) (Trial, error) {
	at := p.Clock.Now()
	if p.Trial == nil {
		failure := &TrialFailure{Code: "backup.trial_restore_unavailable"}
		return Trial{InspectedAt: at, Failure: failure},
			shared.ErrUnavailable.WithDetail(CodeTrialRestoreFailed).
				WithParams(map[string]string{"reason": failure.Code})
	}
	report, err := p.Trial.Inspect(ctx, InspectInput{
		TenantID: in.TenantID, TargetID: in.TargetID, Store: store, Archive: prefix, Report: in.Report,
	})
	if err != nil {
		failure := &TrialFailure{Code: runFailureCode(err), Member: memberOf(err)}
		return Trial{InspectedAt: p.Clock.Now(), Failure: failure},
			shared.ErrValidation.WithDetail(CodeTrialRestoreFailed).
				WithParams(map[string]string{"reason": failure.Code, "member": failure.Member}).
				WithCause(err)
	}
	rendered := reportOutput(report)
	return Trial{InspectedAt: p.Clock.Now(), Report: &rendered}, nil
}

// memberOf is the member a reader's refusal names, where it names one.
func memberOf(err error) string {
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.Params != nil {
		return domainErr.Params["path"]
	}
	return ""
}

// encoded is the trial as the run's row keeps it.
func (t Trial) encoded() []byte {
	raw, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return raw
}
