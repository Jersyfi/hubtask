// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ReadDocumentServerName = "ReadDocumentServer"
	SetDocumentServerName  = "SetDocumentServer"

	// DocumentServerSetAction is the audit code of a changed document server.
	DocumentServerSetAction audit.Action = "media.document_server_set"
	// DocumentServerReadAction is the audit code of a refused read (audit.md §4).
	DocumentServerReadAction audit.Action = "media.document_server_read"

	documentServerTarget = "document_server"
)

// DocumentServers reads and sets the workspace's document server (UC-MED-06): where office files
// are shown and edited. Every member reads it - the viewer asks whether a file can be shown as a
// PDF or edited; the administration sets it under STRUCTURE, which the settings page asks.
type DocumentServers struct {
	Wopi       repository.Wopi
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Read answers the choice.
func (h DocumentServers) Read(ctx context.Context, actor appshared.ActorContext) (media.DocumentServerChoice, error) {
	if err := actor.RequireScope(mediaRead); err != nil {
		return media.DocumentServerChoice{}, err
	}
	var choice media.DocumentServerChoice
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		choice, err = h.Wopi.DocumentServer(ctx)
		return err
	})
	return choice, err
}

// Set writes the choice. An agent never decides it: the server receives the workspace's files.
func (h DocumentServers) Set(
	ctx context.Context, actor appshared.ActorContext, kind, address string,
) (media.DocumentServerChoice, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return media.DocumentServerChoice{}, shared.ErrForbidden.WithDetail("media.agent_decides_document_server_not")
	}
	choice, err := media.NewDocumentServerChoice(media.DocumentServer(kind), address)
	if err != nil {
		return media.DocumentServerChoice{}, err
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: DocumentServerSetAction, TokenScope: mediaWrite,
		TargetType: documentServerTarget, TargetID: actor.TenantID,
	}); err != nil {
		return media.DocumentServerChoice{}, err
	}
	now := h.Clock.Now()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Wopi.SetDocumentServer(ctx, choice, actor.AccountID, now); err != nil {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: DocumentServerSetAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: documentServerTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			// The origin is configuration an administrator typed, not user content (rule 10).
			Changes: audit.Changes(
				audit.Change{Field: "kind", Classification: audit.Open, To: string(choice.Kind)},
				audit.Change{Field: "url", Classification: audit.Open, To: choice.URL},
			),
		})
	})
	return choice, err
}

func documentServerOutput(choice media.DocumentServerChoice) usecase.Output {
	kind := choice.Kind
	if kind == "" {
		kind = media.DocumentServerNone
	}
	out := usecase.Output{"kind": string(kind)}
	if choice.URL != "" {
		out["url"] = choice.URL
	}
	return out
}

// ReadDocumentServer is the read's catalogue entry.
type ReadDocumentServer struct{ Servers DocumentServers }

// Descriptor is the catalogue entry.
func (h ReadDocumentServer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadDocumentServerName, Permission: service.PermissionRead,
		Summary: "Reads where the workspace's office files are shown and edited: none, or a " +
			"document server's kind and its https origin.",
		SideEffects: "None. Reads only.",
		TokenScope:  mediaRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: DocumentServerReadAction, TargetType: documentServerTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's configuration, not a work item; its change is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			choice, err := h.Servers.Read(ctx, actor)
			if err != nil {
				return nil, err
			}
			return documentServerOutput(choice), nil
		}),
	}
}

// SetDocumentServer is the write's catalogue entry.
type SetDocumentServer struct{ Servers DocumentServers }

// Descriptor is the catalogue entry.
func (h SetDocumentServer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetDocumentServerName, Permission: service.PermissionStructure,
		Summary: "Chooses the workspace's document server - NONE, or a kind with its https " +
			"origin - where office files are shown as PDF and edited. Needs STRUCTURE at the " +
			"workspace; an agent never decides it.",
		SideEffects: "Stores the choice and writes an audit entry.",
		TokenScope:  mediaWrite,
		Input: []usecase.Field{
			{Name: "kind", Kind: usecase.KindString, Required: true, Description: "NONE, COLLABORA_ONLINE, ONLYOFFICE, EURO_OFFICE or MICROSOFT_365."},
			{Name: "url", Kind: usecase.KindString, Description: "The server's https origin; absent for NONE."},
		},
		Audit: usecase.AuditDeclaration{Action: DocumentServerSetAction, TargetType: documentServerTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A workspace's configuration, not a work item; its change is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			choice, err := h.Servers.Set(ctx, actor, in.String("kind"), in.String("url"))
			if err != nil {
				return nil, err
			}
			return documentServerOutput(choice), nil
		}),
	}
}
