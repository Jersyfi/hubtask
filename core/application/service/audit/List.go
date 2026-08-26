// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package audit is the trail as somebody reads it: a page of it, an archive of it, and the check
// that it has not been rewritten (E-09, audit.md §5).
//
// Writing is not here and never will be. An entry is written by the use case that caused it,
// inside its transaction, through `core/port/audit.Sink` - and the one action this package writes
// is the export, which is an event in its own right rather than this package auditing somebody
// else's work.
package audit

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListAuditEntriesName = "ListAuditEntries"

	// auditRead and auditExport are the two scopes api-guidelines.md §7 now lists. Two rather than
	// one for the reason the job scopes are two: reading the trail and carrying a copy of it out
	// of the installation are different acts.
	auditRead   = "audit:read"
	auditExport = "audit:export"

	// trailTarget is what an entry about the trail itself names. Its own target type rather than
	// the tenant's, because "somebody read the evidence" is about the evidence.
	trailTarget = "audit_trail"

	// ReadAction is the code a refused read is recorded against.
	//
	// A *successful* read writes nothing, and that is a decision rather than an omission. The
	// trail would otherwise grow by being read - the second page would contain the reading of the
	// first - and §4 does not list reading the trail among the mandatory events. What is recorded
	// is the refusal, which is the entry an auditor is actually looking for, and the export, which
	// is a copy leaving the installation (§5).
	ReadAction port.Action = "audit.read"

	// DefaultPageSize and MaxPageSize are the contract's, restated for this list because the
	// contract states them per operation: `GET /audit` caps at 500 where the work lists cap at
	// 200, because an auditor walking a year of a busy tenant is doing one long read rather than
	// filling a screen.
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// Authorizer is the one place that decides, and the one place that records a refusal (ADR-0005).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	// Permits answers the same question without recording anything. It is what makes the
	// narrowing below possible: "may this actor see the whole trail" has to be asked *before* the
	// decision that is recorded, or a member reading their own events would leave a refusal behind
	// on every request.
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// ListAuditEntries reads a page of the trail.
type ListAuditEntries struct {
	Trail      repository.Trail
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// EntryQuery is the input, typed.
type EntryQuery struct {
	Filter repository.Filter
}

// Page is what one read answers.
type Page struct {
	Records []repository.Record
	Info    repository.PageInfo
}

// Execute answers the page this actor may see.
//
// Two rights rather than one, which is audit.md §5 and not the ordinary role matrix. An `OWNER`, an
// `ADMIN` or an `AUDITOR` reads the whole trail of their tenant. Everybody else reads their own
// events - transparency towards the employee rather than a lesser administrator's view - and the
// narrowing is applied here rather than trusted from the request, because a filter a caller can
// set is a filter a caller can leave out.
//
// A request that names somebody else's events without the right to the whole trail is refused
// rather than quietly narrowed. Silently answering "your own events" to a question about a
// colleague would tell the caller their colleague did nothing, which is a different and false
// answer.
func (h ListAuditEntries) Execute(
	ctx context.Context, actor appshared.ActorContext, query EntryQuery,
) (Page, error) {
	whole, err := h.Authorizer.Permits(ctx, actor, wholeTrail(actor))
	if err != nil {
		return Page{}, err
	}

	request := wholeTrail(actor)
	filter := query.Filter
	if !whole {
		if filter.ActorID.IsZero() || filter.ActorID == actor.AccountID {
			// Their own events, which every member of the workspace may read. The permission
			// asked for is the ordinary one: holding a role here at all is the whole condition.
			request.Permission = service.PermissionRead
			filter.ActorID = actor.AccountID
		}
		// Otherwise the request stays the whole-trail one, so that the refusal below is recorded
		// against reading the trail rather than against something the caller did not ask for.
	}
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return Page{}, err
	}

	if filter.Outcome != "" && !filter.Outcome.Valid() {
		return Page{}, shared.ErrValidation.
			WithDetail("audit.outcome_invalid").
			WithFields(shared.FieldError{Path: "/outcome", Code: "audit.outcome_invalid"})
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
		return Page{}, shared.ErrValidation.
			WithDetail("audit.period_invalid").
			WithFields(shared.FieldError{Path: "/to", Code: "audit.period_invalid"})
	}
	filter.Page.Size = PageSize(filter.Page.Size)

	var page repository.RecordPage
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Trail.Query(ctx, filter)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return Page{Records: page.Records, Info: page.Info}, nil
}

// wholeTrail is the request for the trail of the tenant, which is where a trail lives: it spans
// every hub, and a permission asked for at a container would be a permission nobody could hold
// over the whole of it.
func wholeTrail(actor appshared.ActorContext) access.Request {
	return access.Request{
		Permission: service.PermissionAuditRead,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ReadAction,
		TokenScope: auditRead,
		TargetType: trailTarget,
		TargetID:   actor.TenantID,
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

// Descriptor registers the read in all three channels.
func (h ListAuditEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListAuditEntriesName,
		Summary: "Reads the evidence trail: who did what, when, from where, and whether it " +
			"worked - newest first. Never the content of anything. An owner, an administrator " +
			"or an auditor reads the whole workspace's trail; everybody else reads their own " +
			"events, which is what makes the trail transparent to the people it records rather " +
			"than only to the people above them.",
		SideEffects: "None. Reads only, and a successful read is not itself recorded - a trail " +
			"that grew by being read would bury what it is for. A refused read is recorded.",
		TokenScope: auditRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{
				Name: "from", Kind: usecase.KindString,
				Description: "The start of the period, inclusive. RFC 3339.",
			},
			{
				Name: "to", Kind: usecase.KindString,
				Description: "The end of the period, exclusive. RFC 3339.",
			},
			{
				Name: "action", Kind: usecase.KindString,
				Description: "A prefix of the action code: `auth.` for every authentication " +
					"event, `membership.role_changed` for one kind of event.",
			},
			{
				Name: "actor_id", Kind: usecase.KindID,
				Description: "Whose events. Somebody reading their own trail may name only " +
					"themselves.",
			},
			{
				Name: "target_type", Kind: usecase.KindString,
				Description: "The kind of object the entry is about, e.g. `container` or " +
					"`legal_hold`.",
			},
			{
				Name: "target_id", Kind: usecase.KindID,
				Description: "One object's whole history, whatever was done to it.",
			},
			{
				Name: "outcome", Kind: usecase.KindString,
				Enum:        []string{string(port.OutcomeSuccess), string(port.OutcomeDenied), string(port.OutcomeFailed)},
				Description: "Whether the attempt succeeded, was refused, or failed.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The opaque cursor of the previous page. Omitted starts at the newest entry.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "How many entries to return. Clamped to the contract's maximum.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ReadAction, TargetType: trailTarget,
			Severity: port.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListAuditEntries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	filter := repository.Filter{
		ActionPrefix: in.String("action"),
		TargetType:   in.String("target_type"),
		Outcome:      port.Outcome(in.String("outcome")),
		Page:         repository.Page{Cursor: in.String("cursor"), Size: in.Int("size")},
	}
	if raw := in.String("from"); raw != "" {
		from, err := parseInstant(raw, "from")
		if err != nil {
			return nil, err
		}
		filter.From = from
	}
	if raw := in.String("to"); raw != "" {
		to, err := parseInstant(raw, "to")
		if err != nil {
			return nil, err
		}
		filter.To = to
	}
	if in.Present("actor_id") {
		actorID, err := in.ID("actor_id")
		if err != nil {
			return nil, err
		}
		filter.ActorID = actorID
	}
	if in.Present("target_id") {
		targetID, err := in.ID("target_id")
		if err != nil {
			return nil, err
		}
		filter.TargetID = targetID
	}

	page, err := h.Execute(ctx, actor, EntryQuery{Filter: filter})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Records))
	for _, record := range page.Records {
		rows = append(rows, EntryOutput(record))
	}
	return pageOutput(rows, page.Info), nil
}
