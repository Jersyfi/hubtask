// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

const (
	ReapplyErasuresName = "ReapplyErasures"

	// ErasureReappliedAction is the entry an erasure carried out again after a point-in-time
	// recovery leaves in its workspace's trail. Its own action rather than `dsr.erased`: that entry
	// was rewound away with the erasure, and this one records that the operator carried it out
	// again, and from which recovery.
	ErasureReappliedAction audit.Action = "dsr.erasure_reapplied"

	// reapplyScope is the control plane's scope: the operator's act in the recovery procedure
	// (backup-restore.md §8.5 step 5), across workspaces.
	reapplyScope = "admin:tenants"

	// maxReappliedErasures bounds one call; a workspace with more erasure cases in a rewound period
	// sends them in several calls, each idempotent.
	maxReappliedErasures = 1000

	codeRecoveryPointRequired = "lifecycle.recovery_point_required"
	codeErasuresRequired      = "privacy.erasures_required"
	codeTooManyErasures       = "privacy.too_many_erasures"
	codeErasureRecordInvalid  = "privacy.erasure_record_invalid"
	// CodeErasureRecordMismatch answers a case the recovered workspace contradicts.
	CodeErasureRecordMismatch = "privacy.erasure_record_mismatch"
)

// ReapplyOutcome says what this run did with one case.
type ReapplyOutcome string

const (
	// ErasureReapplied is a case whose erasure this run carried out and whose case it completed.
	ErasureReapplied ReapplyOutcome = "REAPPLIED"
	// ErasureAlreadyCompleted is a case the workspace holds completed already: left as it is.
	ErasureAlreadyCompleted ReapplyOutcome = "ALREADY_COMPLETED"
	// ErasureSubjectAbsent is a case about an account the recovered workspace does not have - one
	// created after the recovery point, or an address nobody held. Completed with nothing to erase.
	ErasureSubjectAbsent ReapplyOutcome = "SUBJECT_ABSENT"
	// ErasureBlocked is an erasure the erasure itself refused, with its code; the case stays open.
	ErasureBlocked ReapplyOutcome = "ERASURE_BLOCKED"
	// ErasureCaseRejected is a case the recovered workspace holds as rejected: a person decides.
	ErasureCaseRejected ReapplyOutcome = "CASE_REJECTED"
	// ErasureCaseMismatch is a case the recovered workspace contradicts, or could not have lost.
	ErasureCaseMismatch ReapplyOutcome = "CASE_MISMATCH"
)

// WorkspaceProbe is the one question asked of the workspace itself: whether it is there.
type WorkspaceProbe interface {
	Find(ctx context.Context) (adminrepo.TenantRecord, error)
}

// ReapplyErasures carries out again the erasures a point-in-time recovery rewound away, after the
// holds of the rewound period are placed again and before traffic is admitted (backup-restore.md
// §8.5 step 5, data-protection.md §5).
//
// The cases arrive with the request, as the operator read them from the cluster recovered to the
// end of the WAL archive; the input is the operator's, and it deletes for good, so it is believed
// only as far as the recovered workspace cannot contradict it. A case the workspace holds must be
// the same erasure - kind, mode, subject - and one it lacks must have been received after the
// recovery point, since only those can a rewind have taken. Each erasure then runs as an erasure
// runs: under the holds in force, so a hold keeps what it covers.
type ReapplyErasures struct {
	Requests   repository.Requests
	Recovered  repository.RecoveredCases
	Eraser     Eraser
	Workspaces WorkspaceProbe
	StepUp     stepup.Verifier
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ReapplyErasuresCommand is the input, typed.
type ReapplyErasuresCommand struct {
	TenantID      shared.ID
	RecoveryPoint time.Time
	// Erasures are the cases as they stood at the end of the archive; Status and Kind are ignored.
	Erasures []domain.Request
	// StepUpToken is the step-up proof (identity.md §16), consumed by this one request.
	StepUpToken string
}

// Reapplied is what happened to one case.
type Reapplied struct {
	ID      shared.ID
	Outcome ReapplyOutcome
	// Code is the message code behind ErasureBlocked and ErasureCaseMismatch.
	Code string
	// Kept is whether a legal hold kept part of what the erasure would have removed.
	Kept bool
}

// Execute carries the erasures out again and answers what happened to each, in the order given.
func (h ReapplyErasures) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ReapplyErasuresCommand,
) ([]Reapplied, error) {
	if err := actor.RequireScope(reapplyScope); err != nil {
		return nil, err
	}
	if cmd.TenantID.IsZero() {
		return nil, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	if cmd.RecoveryPoint.IsZero() {
		return nil, shared.ErrValidation.WithDetail(codeRecoveryPointRequired).
			WithFields(shared.FieldError{Path: "/recovery_point", Code: codeRecoveryPointRequired})
	}
	if err := validErasures(cmd.Erasures); err != nil {
		return nil, err
	}

	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		if _, err := h.Workspaces.Find(ctx); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Outside the workspace's transaction: the proof lives in the operator's own workspace, and a
	// nested scope may not switch tenant.
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
		return nil, err
	}

	// The erasure runs in the target workspace on the operator's behalf: its transaction and its
	// trail are the workspace's, the actor named in the entry is the operator.
	operator := appshared.ActorContext{
		Kind: actor.Kind, TenantID: cmd.TenantID, AccountID: actor.AccountID,
		AccountName: actor.AccountName,
	}
	now := h.Clock.Now()
	results := make([]Reapplied, 0, len(cmd.Erasures))
	for _, record := range cmd.Erasures {
		result, err := h.reapply(ctx, operator, scope, cmd.RecoveryPoint, now, record)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// reapply carries one case out again: prepared in one transaction, erased by the erasure's own,
// completed in a third - each as the ordinary erasure does it.
func (h ReapplyErasures) reapply(
	ctx context.Context, operator appshared.ActorContext, scope persistence.Scope,
	point, now time.Time, record domain.Request,
) (Reapplied, error) {
	result := Reapplied{ID: record.ID}
	mismatch := func() (Reapplied, error) {
		result.Outcome, result.Code = ErasureCaseMismatch, CodeErasureRecordMismatch
		return result, nil
	}
	// Only a case completed between the recovery point and now can a rewind have undone.
	if !record.CompletedAt.After(point) || record.CompletedAt.After(now) {
		return mismatch()
	}

	var subjectPresent bool
	var outcome ReapplyOutcome
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		held, err := h.Requests.Find(ctx, record.ID)
		if errors.Is(err, shared.ErrNotFound) {
			if !record.ReceivedAt.After(point) {
				// Received before the point, the recovered workspace would hold it.
				outcome = ErasureCaseMismatch
				return nil
			}
			inserted, err := h.Recovered.InsertAsItWas(ctx, opened(record))
			if err != nil {
				return err
			}
			if !inserted {
				// The identifier is taken: by another writer since the read, compared like any held
				// case - or by a case of another workspace, which this one cannot see.
				held, err = h.Requests.Find(ctx, record.ID)
				if errors.Is(err, shared.ErrNotFound) {
					outcome = ErasureCaseMismatch
					return nil
				}
				if err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}
		if !held.ID.IsZero() {
			if outcome = against(held, record); outcome != "" {
				return nil
			}
		}
		if !record.SubjectAccountID.IsZero() {
			subjectPresent, err = h.Recovered.SubjectExists(ctx, record.SubjectAccountID)
		}
		return err
	})
	if err != nil {
		return Reapplied{}, err
	}
	switch outcome {
	case ErasureCaseMismatch:
		return mismatch()
	case "":
	default:
		result.Outcome = outcome
		return result, nil
	}

	if !subjectPresent {
		// An account created after the recovery point went with the rewind, and an address case
		// has nobody here: the case is answered, with nothing to erase.
		return h.complete(ctx, scope, record, "", ErasureSubjectAbsent, false)
	}

	erased, err := h.Eraser.erase(ctx, operator, opened(record), ErasureReappliedAction,
		audit.Change{Field: "recovery_point", Classification: audit.Open,
			To: point.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		var refusal *shared.Error
		if errors.As(err, &refusal) && errors.Is(err, shared.ErrConflict) {
			// The erasure's own refusal - a rule still runs as the person, its re-pointing rewound
			// too. The case stays open for its owner; the others go on.
			result.Outcome, result.Code = ErasureBlocked, refusal.DetailCode
			return result, nil
		}
		return Reapplied{}, err
	}
	subject := record.SubjectAccountID
	if record.ErasureMode == domain.ModeFullDelete && !erased.AccountKept && !erased.AccountAnonymised {
		// The account is gone again, and the case stops naming it - as the first completion did.
		subject = ""
	}
	return h.complete(ctx, scope, record, subject, ErasureReapplied, len(pendingParts(erased.Kept)) > 0)
}

// complete closes the case at its first completion's moment, unless somebody completed it first.
func (h ReapplyErasures) complete(
	ctx context.Context, scope persistence.Scope, record domain.Request, subject shared.ID,
	outcome ReapplyOutcome, kept bool,
) (Reapplied, error) {
	result := Reapplied{ID: record.ID, Outcome: outcome, Kept: kept}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		completed, err := h.Recovered.CompleteIfOpen(ctx, record.ID, record.ErasureMode, record.CompletedAt, subject)
		if err != nil {
			return err
		}
		if !completed {
			// The rewound job of the same case finished first; its erasure and this one are the same
			// act, and the case is answered once.
			result.Outcome = ErasureAlreadyCompleted
		}
		return nil
	})
	if err != nil {
		return Reapplied{}, err
	}
	return result, nil
}

// against compares a case the recovered workspace holds with the one sent: empty when the erasure
// may run, otherwise the outcome.
func against(held, record domain.Request) ReapplyOutcome {
	switch {
	case held.Kind != domain.KindErasure:
		return ErasureCaseMismatch
	// A closed case is left as it is whatever was sent: a completed full deletion no longer names
	// its subject, so there is nothing left to compare, and nothing runs.
	case held.Status == domain.StatusCompleted:
		return ErasureAlreadyCompleted
	case held.Status == domain.StatusRejected:
		return ErasureCaseRejected
	case held.ErasureMode != "" && held.ErasureMode != record.ErasureMode:
		return ErasureCaseMismatch
	case held.SubjectAccountID != record.SubjectAccountID:
		return ErasureCaseMismatch
	}
	return ""
}

// opened is the case as the erasure takes it: an open erasure in this workspace.
func opened(record domain.Request) domain.Request {
	record.Kind, record.Status, record.Scope = domain.KindErasure, domain.StatusInProgress, domain.ScopeTenant
	record.CompletedAt = time.Time{}
	return record
}

// validErasures refuses the whole call over a list that is empty, too long, or holds a record that
// names too little: a partial run would leave the operator working out which half went.
func validErasures(records []domain.Request) error {
	switch {
	case len(records) == 0:
		return shared.ErrValidation.WithDetail(codeErasuresRequired).
			WithFields(shared.FieldError{Path: "/erasures", Code: codeErasuresRequired})
	case len(records) > maxReappliedErasures:
		return shared.ErrValidation.WithDetail(codeTooManyErasures).
			WithParams(map[string]string{"maximum": strconv.Itoa(maxReappliedErasures)}).
			WithFields(shared.FieldError{Path: "/erasures", Code: codeTooManyErasures})
	}
	for i, record := range records {
		if record.ID.IsZero() || !record.ErasureMode.Valid() || record.ReceivedAt.IsZero() ||
			record.DueAt.IsZero() || record.CompletedAt.IsZero() ||
			(record.ExtensionReason != "" && !record.ExtensionReason.Valid()) {
			return shared.ErrValidation.WithDetail(codeErasureRecordInvalid).
				WithFields(shared.FieldError{Path: fmt.Sprintf("/erasures/%d", i), Code: codeErasureRecordInvalid})
		}
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h ReapplyErasures) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReapplyErasuresName,
		Summary: "After a point-in-time recovery of the installation, carries out again the " +
			"erasures one workspace completed after the recovery point, each case written again " +
			"where the rewind took it and completed at its own moment. Every erasure runs under the " +
			"legal holds in force. A case the workspace contradicts, or could not have lost, is " +
			"left alone. The operator's, after the holds are placed again and before traffic is " +
			"admitted.",
		SideEffects: "Deletes or anonymises what each erasure reaches, for good; writes the cases it " +
			"lacks and completes them; writes a dsr.erasure_reapplied entry per erasure into the " +
			"workspace's trail.",
		Destructive: true,
		StepUp:      "always - it deletes data for good, from cases the operator supplies",
		TokenScope:  reapplyScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{
				Name: "recovery_point", Kind: usecase.KindString, Format: usecase.FormatDateTime,
				Required:    true,
				Description: "The moment the installation was recovered to, RFC 3339.",
			},
			{
				Name: "erasures", Kind: usecase.KindList, Required: true,
				Description: "The erasure cases completed after the recovery point, at most 1000, " +
					"as the cluster recovered to the end of the WAL archive holds them: id, " +
					"erasure_mode, subject_account_id, received_at, due_at, completed_at and, where " +
					"set, subject_email, handled_by, notes, original_due_at, extension_reason, informed_on.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, consumed by this one request.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ErasureReappliedAction, TargetType: requestTarget,
			Severity: audit.SeverityCritical, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReapplyErasures) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	cmd := ReapplyErasuresCommand{TenantID: tenantID, StepUpToken: in.String("step_up_token")}
	if raw := in.String("recovery_point"); raw != "" {
		if cmd.RecoveryPoint, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			return nil, shared.ErrValidation.WithDetail(codeRecoveryPointRequired).
				WithFields(shared.FieldError{Path: "/recovery_point", Code: codeRecoveryPointRequired})
		}
	}
	list, _ := in["erasures"].([]any)
	for i, item := range list {
		record, err := erasureRecordOf(item)
		if err != nil {
			return nil, shared.ErrValidation.WithDetail(codeErasureRecordInvalid).
				WithFields(shared.FieldError{Path: fmt.Sprintf("/erasures/%d", i), Code: codeErasureRecordInvalid}).
				WithCause(err)
		}
		cmd.Erasures = append(cmd.Erasures, record)
	}

	results, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	rows := make([]any, 0, len(results))
	for _, r := range results {
		row := usecase.Output{"id": r.ID.String(), "outcome": string(r.Outcome), "kept": r.Kept}
		if r.Code != "" {
			row["code"] = r.Code
		}
		rows = append(rows, row)
	}
	return usecase.Output{"erasures": rows}, nil
}

// erasureRecordOf reads one case as the channels deliver it: a JSON object of strings.
func erasureRecordOf(item any) (domain.Request, error) {
	fields, ok := item.(map[string]any)
	if !ok {
		return domain.Request{}, errors.New("an erasure record is not an object")
	}
	text := func(key string) string {
		value, _ := fields[key].(string)
		return value
	}
	id := func(key string) (shared.ID, error) {
		if raw := text(key); raw != "" {
			return shared.ParseID(raw)
		}
		return "", nil
	}
	instant := func(key string) (time.Time, error) {
		if raw := text(key); raw != "" {
			return time.Parse(time.RFC3339Nano, raw)
		}
		return time.Time{}, nil
	}

	var record domain.Request
	var err error
	if record.ID, err = id("id"); err != nil {
		return record, err
	}
	if record.SubjectAccountID, err = id("subject_account_id"); err != nil {
		return record, err
	}
	if record.HandledBy, err = id("handled_by"); err != nil {
		return record, err
	}
	for key, target := range map[string]*time.Time{
		"received_at": &record.ReceivedAt, "due_at": &record.DueAt,
		"completed_at": &record.CompletedAt, "original_due_at": &record.OriginalDueAt,
	} {
		if *target, err = instant(key); err != nil {
			return record, err
		}
	}
	if raw := text("informed_on"); raw != "" {
		if record.InformedOn, err = domain.ParseDay(raw); err != nil {
			return record, err
		}
	}
	record.ErasureMode = domain.ErasureMode(text("erasure_mode"))
	record.SubjectEmail = text("subject_email")
	record.Notes = text("notes")
	record.ExtensionReason = domain.ExtensionReason(text("extension_reason"))
	return record, nil
}
