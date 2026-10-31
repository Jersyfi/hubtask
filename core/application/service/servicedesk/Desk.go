// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package servicedesk is the service desk's use cases (M5): a workspace's business calendars and
// SLA policies (UC-SVC-01, UC-SVC-02), the clocks that measure an entry against its policy, and
// how an entry's goals stand.
//
// Calendars and policies are the workspace's configuration: written with STRUCTURE at the
// workspace, as link types and workflows are, and read by any member. An entry's SLA is read by
// whoever may read the entry.
package servicedesk

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes: the workspace's configuration to write it, the entries' to read it.
const (
	deskWrite = "workspace:manage"
	deskRead  = "items:read"
)

const (
	calendarTarget = "business_calendar"
	policyTarget   = "sla_policy"
	replyTarget    = "reply_template"
)

// The stable audit codes (audit.md §2). Names are a person's words and go as fingerprints
// (rule 10).
const (
	CalendarDefinedAction audit.Action = "business_calendar.defined"
	CalendarDeletedAction audit.Action = "business_calendar.deleted"
	CalendarsReadAction   audit.Action = "business_calendar.read"
	PolicyDefinedAction   audit.Action = "sla_policy.defined"
	PolicyDeletedAction   audit.Action = "sla_policy.deleted"
	PoliciesReadAction    audit.Action = "sla_policy.read"
	EntrySLAReadAction    audit.Action = "entry_sla.read"
	SLAReportReadAction   audit.Action = "entry_sla.reported"
	ReplyDefinedAction    audit.Action = "reply_template.defined"
	ReplyDeletedAction    audit.Action = "reply_template.deleted"
	RepliesReadAction     audit.Action = "reply_template.read"
	ReplyRenderedAction   audit.Action = "reply_template.rendered"
)

const configurationExempt = "a calendar, an SLA policy and a reply template are the workspace's configuration; the entries they measure record their own history."

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Desk is what every calendar and policy use case shares.
type Desk struct {
	Calendars repository.Calendars
	// Offered are the installation's calendars (WithInstallation): a policy may name one, and a
	// workspace calendar of the same key may go though a policy names it.
	Offered  map[string]domain.Calendar
	Policies repository.Policies
	Records  repository.Records
	Replies  repository.ReplyTemplates
	Ratings  repository.Ratings
	// Organisations groups the portal's requesters by mail domain (UC-SVC-11); nil holds none.
	Organisations repository.Organisations
	// HubCalendars and Containers serve a hub's own calendars: the hub is read to ask there.
	HubCalendars repository.HubCalendars
	Containers   workrepo.Containers
	Authorizer   Authorizer
	Audit        audit.Sink
	UnitOfWork   persistence.UnitOfWork
	Clock        clock.Clock
	IDs          clock.IDGenerator
	Text         text.Normalizer
}

func (d Desk) authorizeWrite(ctx context.Context, actor appshared.ActorContext, action audit.Action, target string) error {
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: deskWrite, TargetType: target, TargetID: actor.TenantID,
	})
}

// authorizeRead asks READ at the workspace: the calendars and policies are the workspace's
// configuration, read by its members and not by a requester (service.IsRequester).
func (d Desk) authorizeRead(ctx context.Context, actor appshared.ActorContext, action audit.Action, target string) error {
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: deskRead, TargetType: target, TargetID: actor.TenantID,
	})
}

func (d Desk) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID,
	now time.Time, changes ...audit.Change,
) error {
	return d.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// descriptor is the shape every service desk use case shares.
func descriptor(d usecase.Descriptor, target string) usecase.Descriptor {
	d.Module = module.ServiceDesk
	if d.Audit.Action != "" {
		d.Audit.TargetType, d.Audit.Severity, d.Audit.Required = target, audit.SeverityInfo, !d.ReadOnly
	}
	if d.Activity.Exempt == "" && d.Activity.Verb == "" {
		d.Activity.Exempt = configurationExempt
		if d.ReadOnly {
			d.Activity.Exempt = "a read changes nothing."
		}
	}
	if d.Permission == "" && !d.ReadOnly {
		d.Permission = service.PermissionStructure
	}
	return d
}
