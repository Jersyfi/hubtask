// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package privacy is data subject rights as use cases rather than as a support process
// (data-protection.md §4, ADR-0018 decision 4).
//
// What lives here is the case: recording it, listing what is owed, and moving it along. What the
// *work* is - the archive an access request produces, the erasure that serves every storage
// location in the data catalogue - lives beside it, because those are questions about storage and
// this is a question about a legal obligation.
package privacy

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateDataSubjectRequestName = "CreateDataSubjectRequest"
	ListDataSubjectRequestsName  = "ListDataSubjectRequests"
	UpdateDataSubjectRequestName = "UpdateDataSubjectRequest"

	// The scopes api-guidelines.md §7 now lists. Two, for the reason the audit's two are two:
	// reading which cases are open and *acting* on somebody's data are different acts.
	privacyRead   = "privacy:read"
	privacyManage = "privacy:manage"
	// instanceScope is the credential bound on the one operation that crosses the tenant
	// boundary. It is the scope api-guidelines.md §7 has listed since phase 0 for exactly this
	// kind of act.
	instanceScope = "admin:tenants"

	requestTarget = "data_subject_request"

	// The actions the trail records. `dsr.` because that is the prefix `audit.md` §2 names for
	// the occasion of a privacy-relevant entry - `legal_basis: dsr.erasure` - and an action and an
	// occasion that spelled the same thing differently would be two vocabularies.
	RequestRecordedAction  audit.Action = "dsr.recorded"
	RequestStartedAction   audit.Action = "dsr.started"
	RequestCompletedAction audit.Action = "dsr.completed"
	RequestRejectedAction  audit.Action = "dsr.rejected"

	// DefaultPageSize and MaxPageSize are the contract's for this list.
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// Authorizer is the one place that decides, and the one place that records a refusal (ADR-0005).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Enqueuer is the half of the queue a case needs: starting one asks for work and never claims any.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Cases is what the three case use cases share.
type Cases struct {
	Requests   repository.Requests
	Jobs       Enqueuer
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// CreateDataSubjectRequest records a right somebody has exercised.
type CreateDataSubjectRequest struct{ Cases Cases }

// ListDataSubjectRequests answers what this workspace still owes.
type ListDataSubjectRequests struct{ Cases Cases }

// UpdateDataSubjectRequest moves a case along, and is what starts the work.
type UpdateDataSubjectRequest struct{ Cases Cases }

// CreateCommand is the input, typed.
type CreateCommand struct {
	Kind             domain.Kind
	Scope            domain.Scope
	SubjectAccountID shared.ID
	SubjectEmail     string
	DueAt            time.Time
	TargetID         shared.ID
	Notes            string
}

// Execute records the case.
//
// `MANAGE_MEMBERS`, because a data subject request is about a *person* rather than about the shape
// of the workspace: the administrator who grants and revokes access is the one who answers for it
// (domain-model.md §5). Recording a case destroys nothing and exports nothing - what needs the
// owner's right is starting an erasure, and that is asked for where the case starts.
func (h CreateDataSubjectRequest) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateCommand,
) (domain.Request, error) {
	if err := h.Cases.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RequestRecordedAction,
		TokenScope: privacyManage,
		TargetType: requestTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return domain.Request{}, err
	}
	if cmd.Scope == domain.ScopeInstallation {
		if err := requireInstanceScope(actor); err != nil {
			return domain.Request{}, err
		}
	}

	request, err := domain.NewRequest(domain.NewRequestInput{
		ID: h.Cases.IDs.NewID(), Kind: cmd.Kind, Scope: cmd.Scope,
		SubjectAccountID: cmd.SubjectAccountID, SubjectEmail: cmd.SubjectEmail,
		DueAt: cmd.DueAt, TargetID: cmd.TargetID, Notes: cmd.Notes,
		Now: h.Cases.Clock.Now(),
	})
	if err != nil {
		return domain.Request{}, err
	}

	err = h.Cases.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Cases.Requests.Insert(ctx, request); err != nil {
			return err
		}
		return h.Cases.record(ctx, actor, RequestRecordedAction, request, audit.SeverityNotice,
			[]audit.Change{
				{Field: "kind", Classification: audit.Open, To: string(request.Kind)},
				{Field: "scope", Classification: audit.Open, To: string(request.Scope)},
				{
					Field: "due_at", Classification: audit.Open,
					To: request.DueAt.UTC().Format(time.RFC3339),
				},
			})
	})
	if err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

// ListQuery is the input, typed.
type ListQuery struct {
	Status        domain.Status
	Kind          domain.Kind
	DueWithinDays int
	IncludeClosed bool
	Cursor        string
	Size          int
}

// Execute answers one page of the cases.
//
// The same permission the recording needs, and deliberately not a lesser one: the list says who
// asked for what and when it is due, which is a person's exercised right rather than an
// administrative statistic.
func (h ListDataSubjectRequests) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListQuery,
) (repository.Page, error) {
	if err := h.Cases.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     RequestRecordedAction,
		TokenScope: privacyRead,
		TargetType: requestTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return repository.Page{}, err
	}
	if query.Status != "" && !statusKnown(query.Status) {
		return repository.Page{}, shared.ErrValidation.
			WithDetail(domain.CodeTransitionRefused).
			WithFields(shared.FieldError{Path: "/status", Code: domain.CodeTransitionRefused})
	}
	if query.Kind != "" && !query.Kind.Valid() {
		return repository.Page{}, shared.ErrValidation.
			WithDetail(domain.CodeKindInvalid).
			WithFields(shared.FieldError{Path: "/kind", Code: domain.CodeKindInvalid})
	}

	filter := repository.Filter{
		Status: query.Status, Kind: query.Kind, IncludeClosed: query.IncludeClosed,
		Cursor: query.Cursor, Size: PageSize(query.Size),
	}
	if query.DueWithinDays > 0 {
		// Overdue ones included: "what falls due in the next seven days" is a question about work
		// to do, and a case that is already late is the most urgent of it.
		filter.DueBefore = h.Cases.Clock.Now().AddDate(0, 0, query.DueWithinDays)
	}

	var page repository.Page
	err := h.Cases.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Cases.Requests.List(ctx, filter)
		return err
	})
	if err != nil {
		return repository.Page{}, err
	}
	return page, nil
}

// UpdateCommand is the input, typed. Every field is optional; what is absent is left alone.
type UpdateCommand struct {
	RequestID       shared.ID
	Status          domain.Status
	ErasureMode     domain.ErasureMode
	HandledBy       shared.ID
	RejectionReason string
	Notes           string
	TargetID        shared.ID
}

// Execute moves the case along.
//
// This is where the work starts, and where the permission changes with it. Assigning a case,
// noting something on it or refusing it is the administrator's; **starting an erasure asks for the
// owner's right**, because it destroys work - the same line a retention rule and a legal hold sit
// on (domain-model.md §5). Starting an export does not: it writes an archive to a target somebody
// has already approved, and using an approved channel is running the workspace.
func (h UpdateDataSubjectRequest) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateCommand,
) (domain.Request, error) {
	if cmd.RequestID.IsZero() {
		return domain.Request{}, shared.ErrValidation.WithDetail(domain.CodeRequestNotFound)
	}

	// The case has to be read before the permission can be decided: what is being asked for
	// depends on the kind, and "may this actor start an erasure" is a different question from "may
	// they note something on a rectification". The read is its own transaction and the decision
	// happens before the one that writes, so a refusal leaves nothing behind (ADR-0005).
	var current domain.Request
	if err := h.Cases.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		current, err = h.Cases.Requests.Find(ctx, cmd.RequestID)
		return err
	}); err != nil {
		return domain.Request{}, err
	}

	action, permission := RequestStartedAction, service.PermissionManageMembers
	switch {
	case cmd.Status == domain.StatusInProgress && current.Kind == domain.KindErasure:
		// The owner's line. Somebody who can erase a person from a workspace is destroying work
		// that belongs to the workspace as much as to them.
		permission = service.PermissionDeleteContainer
	case cmd.Status == domain.StatusRejected:
		action = RequestRejectedAction
	case cmd.Status == domain.StatusCompleted:
		action = RequestCompletedAction
	}

	if err := h.Cases.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: privacyManage,
		TargetType: requestTarget,
		TargetID:   cmd.RequestID,
	}); err != nil {
		return domain.Request{}, err
	}
	if current.Scope == domain.ScopeInstallation {
		if err := requireInstanceScope(actor); err != nil {
			return domain.Request{}, err
		}
	}

	now := h.Cases.Clock.Now()
	moved, changes, err := h.apply(current, cmd, actor, now)
	if err != nil {
		return domain.Request{}, err
	}

	err = h.Cases.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		saved, err := h.Cases.Requests.Save(ctx, moved)
		if err != nil {
			return err
		}
		if !saved {
			return shared.ErrNotFound.WithDetail(domain.CodeRequestNotFound)
		}

		if moved.Status == domain.StatusInProgress && needsWork(moved.Kind) {
			if _, err := h.Cases.Jobs.Enqueue(ctx, queue.Request{
				Kind:      queue.KindPrivacyRequest,
				TenantID:  actor.TenantID,
				Payload:   map[string]any{"request_id": moved.ID.String()},
				DedupeKey: "dsr:" + moved.ID.String(),
			}); err != nil {
				return err
			}
		}

		severity := audit.SeverityNotice
		if moved.Kind == domain.KindErasure && moved.Status == domain.StatusInProgress {
			// The one entry in this file that is a warning: an erasure that has started is a
			// deletion nobody can undo.
			severity = audit.SeverityWarning
		}
		return h.Cases.record(ctx, actor, action, moved, severity, changes)
	})
	if err != nil {
		return domain.Request{}, err
	}
	return moved, nil
}

// apply is the domain's decision about the step, and the changes the trail records for it.
func (h UpdateDataSubjectRequest) apply(
	current domain.Request, cmd UpdateCommand, actor appshared.ActorContext, now time.Time,
) (domain.Request, []audit.Change, error) {
	moved := current
	if cmd.Notes != "" {
		moved.Notes = cmd.Notes
	}
	if !cmd.HandledBy.IsZero() {
		moved.HandledBy = cmd.HandledBy
	}
	if !cmd.TargetID.IsZero() {
		moved.TargetID = cmd.TargetID
	}
	if cmd.ErasureMode != "" {
		if !cmd.ErasureMode.Valid() {
			return domain.Request{}, nil, shared.ErrValidation.
				WithDetail(domain.CodeErasureModeRequired).
				WithFields(shared.FieldError{
					Path: "/erasure_mode", Code: domain.CodeErasureModeRequired,
				})
		}
		moved.ErasureMode = cmd.ErasureMode
	}

	changes := []audit.Change{
		{Field: "kind", Classification: audit.Open, To: string(current.Kind)},
	}
	if cmd.Status == "" {
		return moved, changes, nil
	}

	changes = append(changes, audit.Change{
		Field: "status", Classification: audit.Open,
		From: string(current.Status), To: string(cmd.Status),
	})

	switch cmd.Status {
	case domain.StatusInProgress:
		started, err := moved.Start(cmd.ErasureMode, cmd.TargetID, actor.AccountID)
		if err != nil {
			return domain.Request{}, nil, err
		}
		if started.ErasureMode != "" {
			changes = append(changes, audit.Change{
				Field: "erasure_mode", Classification: audit.Open, To: string(started.ErasureMode),
			})
		}
		return started, changes, nil
	case domain.StatusRejected:
		rejected, err := moved.Reject(cmd.RejectionReason, actor.AccountID, now)
		if err != nil {
			return domain.Request{}, nil, err
		}
		// The reason is the whole point of the entry: a refusal an auditor cannot evaluate is not
		// an answer. It is the operator's own words rather than the person's data.
		changes = append(changes, audit.Change{
			Field: "rejection_reason", Classification: audit.Open, To: rejected.RejectionReason,
		})
		return rejected, changes, nil
	case domain.StatusCompleted:
		// A case somebody completes by hand: a rectification, which needs no special path, or one
		// whose work happened outside this system. The kinds that produce work are completed by
		// the job that did it.
		done, err := moved.Complete(now, "")
		if err != nil {
			return domain.Request{}, nil, err
		}
		return done, changes, nil
	default:
		return domain.Request{}, nil, shared.ErrValidation.
			WithDetail(domain.CodeTransitionRefused).
			WithParams(map[string]string{"from": string(current.Status), "to": string(cmd.Status)}).
			WithFields(shared.FieldError{Path: "/status", Code: domain.CodeTransitionRefused})
	}
}

// needsWork reports whether starting the case sets a job going. Restriction, objection and
// rectification are answered by other use cases or by an ordinary write, and a job for them would
// be a job with nothing to do.
func needsWork(kind domain.Kind) bool {
	return kind == domain.KindErasure || kind.ProducesArchive()
}

// requireInstanceScope is the credential bound on the one operation that crosses the tenant
// boundary. The role says what somebody may do in *this* workspace, and no role in any workspace
// can answer for another one - so the bound is the credential, and it is checked here rather than
// inferred from a permission that does not mean this.
func requireInstanceScope(actor appshared.ActorContext) error {
	if actor.HasScope(instanceScope) {
		return nil
	}
	return shared.ErrForbidden.
		WithDetail(domain.CodeInstallationScopeDenied).
		WithParams(map[string]string{"scope": instanceScope})
}

func statusKnown(status domain.Status) bool {
	switch status {
	case domain.StatusReceived, domain.StatusInProgress,
		domain.StatusCompleted, domain.StatusRejected:
		return true
	default:
		return false
	}
}

// PageSize clamps a requested size into this operation's range.
func PageSize(requested int) int {
	switch {
	case requested < 1:
		return DefaultPageSize
	case requested > MaxPageSize:
		return MaxPageSize
	default:
		return requested
	}
}

// record writes the entry a case owes.
//
// `legal_basis` is set on every one of them, which is the field `audit.md` §2 has carried for this
// since phase 0 and nothing has ever written: a privacy-relevant entry names its occasion, and the
// occasion of these is the right that was exercised.
func (c Cases) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	request domain.Request, severity audit.Severity, changes []audit.Change,
) error {
	return c.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: c.Clock.Now(),
		Action: action, Outcome: audit.OutcomeSuccess, Severity: severity,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: requestTarget, TargetID: request.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
		LegalBasis: LegalBasisOf(request.Kind),
	})
}

// LegalBasisOf is the occasion an entry names, in the vocabulary `audit.md` §2 gives as its
// example: `dsr.erasure`.
func LegalBasisOf(kind domain.Kind) string {
	switch kind {
	case domain.KindErasure:
		return "dsr.erasure"
	case domain.KindAccess:
		return "dsr.access"
	case domain.KindPortability:
		return "dsr.portability"
	case domain.KindRestriction:
		return "dsr.restriction"
	case domain.KindObjection:
		return "dsr.objection"
	case domain.KindRectification:
		return "dsr.rectification"
	default:
		return "dsr"
	}
}

// Descriptor registers recording a case in all three channels.
func (h CreateDataSubjectRequest) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateDataSubjectRequestName,
		Summary: "Records a right somebody has exercised - access, erasure, portability, " +
			"restriction, objection or rectification - as a case with the statutory deadline it " +
			"has to be answered within. Nothing is collected, exported or erased by this: what " +
			"happens next is the controller's decision, taken by moving the case along.",
		SideEffects: "Writes the case and an audit entry naming the right, the scope and the " +
			"deadline. No data of the person's is touched.",
		TokenScope: privacyManage,
		Input: []usecase.Field{
			{
				Name: "kind", Kind: usecase.KindString, Required: true,
				Enum: []string{
					string(domain.KindAccess), string(domain.KindErasure),
					string(domain.KindPortability), string(domain.KindRestriction),
					string(domain.KindObjection), string(domain.KindRectification),
				},
				Description: "Which right was exercised.",
			},
			{
				Name: "scope", Kind: usecase.KindString,
				Enum: []string{string(domain.ScopeTenant), string(domain.ScopeInstallation)},
				Description: "How far the case reaches. This workspace by default; every " +
					"workspace of the installation the person is a member of needs the " +
					"admin:tenants scope.",
			},
			{
				Name: "subject_account_id", Kind: usecase.KindID,
				Description: "The account the case is about.",
			},
			{
				Name: "subject_email", Kind: usecase.KindString,
				Description: "Who asked, where there is no account behind the request.",
			},
			{
				Name: "due_at", Kind: usecase.KindString,
				Description: "The deadline, when it is not thirty days from receipt. RFC 3339.",
			},
			{
				Name: "target_id", Kind: usecase.KindID,
				Description: "The backup target an access or portability export is written to.",
			},
			{Name: "notes", Kind: usecase.KindString, Description: "What is known about the request."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RequestRecordedAction, TargetType: requestTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateDataSubjectRequest) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := CreateCommand{
		Kind:         domain.Kind(in.String("kind")),
		Scope:        domain.Scope(in.String("scope")),
		SubjectEmail: in.String("subject_email"),
		Notes:        in.String("notes"),
	}
	for field, into := range map[string]*shared.ID{
		"subject_account_id": &cmd.SubjectAccountID,
		"target_id":          &cmd.TargetID,
	} {
		id, err := in.ID(field)
		if err != nil {
			return nil, err
		}
		*into = id
	}
	if raw := in.String("due_at"); raw != "" {
		due, err := parseInstant(raw, "due_at")
		if err != nil {
			return nil, err
		}
		cmd.DueAt = due
	}

	request, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return RequestOutput(request), nil
}

// Descriptor registers the listing.
func (h ListDataSubjectRequests) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListDataSubjectRequestsName,
		Summary: "The data subject requests this workspace is handling, the soonest deadline " +
			"first - which is the order the work has to be done in. The open ones by default, " +
			"because \"what do we still owe\" is the question; the answered and refused ones " +
			"when asked for.",
		SideEffects: "None. Reads only.",
		TokenScope:  privacyRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "status", Kind: usecase.KindString,
				Enum: []string{
					string(domain.StatusReceived), string(domain.StatusInProgress),
					string(domain.StatusCompleted), string(domain.StatusRejected),
				},
				Description: "Only the cases in that state.",
			},
			{
				Name: "kind", Kind: usecase.KindString,
				Enum: []string{
					string(domain.KindAccess), string(domain.KindErasure),
					string(domain.KindPortability), string(domain.KindRestriction),
					string(domain.KindObjection), string(domain.KindRectification),
				},
				Description: "Only that right.",
			},
			{
				Name: "due_within_days", Kind: usecase.KindInt,
				Description: "Only the cases falling due inside that many days, overdue ones included.",
			},
			{
				Name: "include_closed", Kind: usecase.KindBool,
				Description: "Show the answered and refused cases as well.",
			},
			{Name: "cursor", Kind: usecase.KindString, Description: "The opaque cursor of the previous page."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many cases to return."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RequestRecordedAction, TargetType: requestTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListDataSubjectRequests) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	page, err := h.Execute(ctx, actor, ListQuery{
		Status:        domain.Status(in.String("status")),
		Kind:          domain.Kind(in.String("kind")),
		DueWithinDays: in.Int("due_within_days"),
		IncludeClosed: in.Present("include_closed") && in.Bool("include_closed"),
		Cursor:        in.String("cursor"),
		Size:          in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Requests))
	for _, request := range page.Requests {
		rows = append(rows, RequestOutput(request))
	}
	return pageOutput(rows, page.Info), nil
}

// Descriptor registers moving a case along.
func (h UpdateDataSubjectRequest) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateDataSubjectRequestName,
		Summary: "Assigns a case, notes something on it, or moves it: to `IN_PROGRESS`, which is " +
			"what starts the work, to `COMPLETED`, or to `REJECTED` with the reason that makes " +
			"a refusal an answer. An illegitimate step is refused by name rather than ignored.",
		SideEffects: "Writes the case and an audit entry. Starting an access or portability case " +
			"queues the archive; starting an erasure queues a deletion nobody can undo.",
		TokenScope:  privacyManage,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "request_id", Kind: usecase.KindID, Required: true, Description: "Which case."},
			{
				Name: "status", Kind: usecase.KindString,
				Enum: []string{
					string(domain.StatusInProgress), string(domain.StatusCompleted),
					string(domain.StatusRejected),
				},
				Description: "Where the case moves to.",
			},
			{
				Name: "erasure_mode", Kind: usecase.KindString,
				Enum:        []string{string(domain.ModeAnonymize), string(domain.ModeFullDelete)},
				Description: "How an erasure is to be carried out. Required before one can start.",
			},
			{Name: "handled_by", Kind: usecase.KindID, Description: "Who is answering for the case."},
			{
				Name: "rejection_reason", Kind: usecase.KindString,
				Description: "Why the request is refused. Required when it is.",
			},
			{
				Name: "target_id", Kind: usecase.KindID,
				Description: "The backup target an export is written to.",
			},
			{Name: "notes", Kind: usecase.KindString, Description: "What is known about the case."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RequestStartedAction, TargetType: requestTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateDataSubjectRequest) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd := UpdateCommand{
		Status:          domain.Status(in.String("status")),
		ErasureMode:     domain.ErasureMode(in.String("erasure_mode")),
		RejectionReason: in.String("rejection_reason"),
		Notes:           in.String("notes"),
	}
	for field, into := range map[string]*shared.ID{
		"request_id": &cmd.RequestID,
		"handled_by": &cmd.HandledBy,
		"target_id":  &cmd.TargetID,
	} {
		id, err := in.ID(field)
		if err != nil {
			return nil, err
		}
		*into = id
	}

	request, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return RequestOutput(request), nil
}
