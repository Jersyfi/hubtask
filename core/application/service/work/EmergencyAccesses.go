// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListPrivateHubAccessesName     = "ListPrivateHubAccesses"
	ListEmergencyNoticesName       = "ListEmergencyNotices"
	AcknowledgeEmergencyNoticeName = "AcknowledgeEmergencyNotice"

	// PrivateHubAccessesReadAction is the code a refused read of a hub's accesses is recorded under.
	PrivateHubAccessesReadAction audit.Action = "container.emergency_accesses_read"

	// EmergencyNoticesReadAction is declared for the catalogue; reading one's own notices refuses
	// nobody anything beyond the credential, so nothing is recorded under it.
	EmergencyNoticesReadAction audit.Action = "container.emergency_notices_read"

	// EmergencyNoticeAcknowledgedAction is the trail's entry for a member's confirmation that they
	// read the notice of an access (UC-ID-16 check 10): the hub and the access, never the reason.
	EmergencyNoticeAcknowledgedAction audit.Action = "container.emergency_notice_acknowledged"

	// EmergencyNoticeDays is how long the app shows a member that their hub was opened.
	EmergencyNoticeDays = 30

	// accessesShown bounds a hub's list: a hub opened more often than this has a story the trail
	// tells, and a list that long is not something a member reads.
	accessesShown = 200
)

// ListPrivateHubAccesses answers every emergency access to a hub, newest first - the members'
// record of who opened their hub, when and why (ADR-0073 §4). Whoever reads the hub reads it: its
// members, and the owner while their access runs; for everybody else the hub is not there.
type ListPrivateHubAccesses struct {
	Accesses   repository.EmergencyAccesses
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute lists them.
func (h ListPrivateHubAccesses) Execute(
	ctx context.Context, actor appshared.ActorContext, hubID shared.ID,
) ([]domain.EmergencyAccess, error) {
	if hubID.IsZero() {
		return nil, containerIDRequired()
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)},
		Action:     PrivateHubAccessesReadAction,
		TokenScope: containersRead,
		TargetType: containerTarget,
		TargetID:   hubID,
	}); err != nil {
		return nil, err
	}
	var accesses []domain.EmergencyAccess
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		accesses, err = h.Accesses.List(ctx, hubID, accessesShown)
		return err
	})
	return accesses, err
}

// Descriptor is the catalogue entry.
func (h ListPrivateHubAccesses) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListPrivateHubAccessesName,
		Permission: service.PermissionRead,
		Summary: "Lists every emergency access to a private hub, newest first: who opened it, when, " +
			"until when and the reason they gave. For whoever reads the hub.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PrivateHubAccessesReadAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			accesses, err := h.Execute(ctx, actor, hubID)
			if err != nil {
				return nil, err
			}
			return accessesOutput(accesses), nil
		}),
	}
}

// ListEmergencyNotices answers the person's own notices of the last 30 days: each time the
// workspace's owner opened a private hub they are a member of, from the record written when it was
// opened, whatever the mail about it did. It is how a person without a mailbox - a child's managed
// account - or one whose mail failed is told (ADR-0073 §4, identity.md §22). Not a list of
// notifications: one kind of notice, and nothing to mark read.
type ListEmergencyNotices struct {
	Accesses   repository.EmergencyAccesses
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute lists them.
func (h ListEmergencyNotices) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.EmergencyNotice, error) {
	if !actor.IsAuthenticated() {
		return nil, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(containersRead); err != nil {
		return nil, err
	}
	if actor.Kind != appshared.ActorUser {
		// Nobody tells a service account or an agent that a hub was opened: only people are a
		// hub's members.
		return []domain.EmergencyNotice{}, nil
	}
	since := h.Clock.Now().Add(-EmergencyNoticeDays * 24 * time.Hour)
	var notices []domain.EmergencyNotice
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		notices, err = h.Accesses.NoticesFor(ctx, actor.AccountID, since)
		return err
	})
	return notices, err
}

// Descriptor is the catalogue entry.
func (h ListEmergencyNotices) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListEmergencyNoticesName,
		Summary: "Lists the emergency accesses the caller was told of in the last 30 days: each time " +
			"the workspace's owner opened a private hub the caller is a member of, with who, when, " +
			"until when and why, and when the caller confirmed it.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			// The caller's own records: nothing to refuse beyond the credential, nothing to record.
			Action: EmergencyNoticesReadAction, TargetType: "account",
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			notices, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(notices))
			for _, notice := range notices {
				data = append(data, emergencyNoticeOutput(notice))
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// AcknowledgeEmergencyNotice records that a member read the notice of an emergency access
// (UC-ID-16 check 10): once, by a person who was told of it within the days the list shows it, by
// the same records the list reads. Anybody else - the owner who opened the hub, a member of another
// hub, a service account - is answered not found, as if there were no such access. Twice is once:
// a repeat writes neither the row nor the trail's entry. Who confirmed is in the trail only; the
// hub's list of accesses does not say it (ADR-0073 §4).
type AcknowledgeEmergencyNotice struct {
	Accesses   repository.EmergencyAccesses
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute records it.
func (h AcknowledgeEmergencyNotice) Execute(
	ctx context.Context, actor appshared.ActorContext, accessID shared.ID,
) error {
	if !actor.IsAuthenticated() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(containersWrite); err != nil {
		return err
	}
	if actor.Kind != appshared.ActorUser || actor.AccountID.IsZero() {
		// Only people are told, so only a person has anything to confirm.
		return domain.ErrEmergencyNoticeNotFound()
	}
	now := h.Clock.Now()
	since := now.Add(-EmergencyNoticeDays * 24 * time.Hour)
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		notices, err := h.Accesses.NoticesFor(ctx, actor.AccountID, since)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(notices, func(n domain.EmergencyNotice) bool { return n.Access.ID == accessID })
		if index < 0 {
			return domain.ErrEmergencyNoticeNotFound()
		}
		notice := notices[index]
		if notice.Acknowledged() {
			return nil
		}
		written, err := h.Accesses.Acknowledge(ctx, accessID, actor.AccountID, now)
		if err != nil || !written {
			// Not written: a confirmation that ran beside this one wrote the row and its entry.
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     EmergencyNoticeAcknowledgedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: containerTarget,
			TargetID:   notice.Access.HubID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			// The access by its identifier alone: its reason is written to the members, and the
			// trail is read by people who are not them (rule 10).
			Changes: audit.Changes(
				audit.Change{Field: "access_id", Classification: audit.Open, To: accessID.String()},
			),
		})
	})
}

// Descriptor is the catalogue entry.
func (h AcknowledgeEmergencyNotice) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AcknowledgeEmergencyNoticeName,
		Summary: "Records that the caller read the notice of an emergency access they were told of in " +
			"the last 30 days. Confirming twice is confirming once; an access the caller was not told " +
			"of is not found.",
		SideEffects: "Stores the caller's confirmation and writes an audit entry, the first time only.",
		TokenScope:  containersWrite,
		Input: []usecase.Field{
			{Name: "access_id", Kind: usecase.KindID, Required: true, Description: "The emergency access."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EmergencyNoticeAcknowledgedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true, FirstTimeOnly: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read receipt of one's own notice."},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			accessID, err := in.ID("access_id")
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, h.Execute(ctx, actor, accessID)
		}),
	}
}

// emergencyNoticeOutput is one access as the member told of it reads it.
func emergencyNoticeOutput(notice domain.EmergencyNotice) usecase.Output {
	out := emergencyAccessOutput(notice.Access)
	var acknowledged any
	if notice.Acknowledged() {
		acknowledged = notice.AcknowledgedAt
	}
	out["acknowledged_at"] = acknowledged
	return out
}

func accessesOutput(accesses []domain.EmergencyAccess) usecase.Output {
	data := make([]usecase.Output, 0, len(accesses))
	for _, each := range accesses {
		data = append(data, emergencyAccessOutput(each))
	}
	return usecase.Output{"data": data}
}
