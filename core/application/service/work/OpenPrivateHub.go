// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"time"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
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
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	OpenPrivateHubName = "OpenPrivateHub"

	// EmergencyAccessOpenedAction is the trail's entry for the workspace owner's opening of a
	// private hub (ADR-0073 §4, UC-ID-16 check 4).
	EmergencyAccessOpenedAction audit.Action = "container.emergency_access_opened"
)

// EmergencyNotices tells a private hub's members that it was opened - the notification service's
// RecordEmergencyAccess - inside the caller's transaction.
type EmergencyNotices interface {
	EmergencyAccessOpened(ctx context.Context, access domain.EmergencyAccess, members []shared.ID) error
}

// OpenPrivateHub is the workspace owner's emergency access to a private hub (ADR-0073 §4): a fresh
// proof, a stated reason, one hour, not renewable - and every member of the hub told in the same
// transaction that opens it, so that nobody reads the hub before its members are told.
//
// Only the workspace's owner - a person, never a service account or a rule - and only for a private
// hub they hold nothing in: a shared hub is theirs to read already, and one they hold a role in is
// open to them. A second access while one runs is refused, naming its end; one after it ended is a
// new act, told again. What it opens, and for whom, is the authoriser's (identity.md §22).
type OpenPrivateHub struct {
	Accesses   repository.EmergencyAccesses
	Hubs       identityrepository.HubLocks
	Notices    EmergencyNotices
	Authorizer Authorizer
	// StepUp judges the fresh proof. Nil refuses rather than letting the act through unproven.
	StepUp     stepup.Verifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// OpenPrivateHubCommand is the input, typed.
type OpenPrivateHubCommand struct {
	HubID       shared.ID
	Reason      string
	StepUpToken string
}

// Execute opens the hub and answers the access.
func (h OpenPrivateHub) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd OpenPrivateHubCommand,
) (domain.EmergencyAccess, error) {
	if cmd.HubID.IsZero() {
		return domain.EmergencyAccess{}, containerIDRequired()
	}
	if err := h.authorize(ctx, actor, cmd.HubID); err != nil {
		return domain.EmergencyAccess{}, err
	}
	opening, err := domain.NewEmergencyAccess(domain.NewEmergencyAccessInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID, HubID: cmd.HubID, OpenedBy: actor.AccountID,
		Reason: cmd.Reason, At: h.Clock.Now(),
	})
	if err != nil {
		return domain.EmergencyAccess{}, err
	}

	// Asked before the proof is spent, so that a hub that cannot be opened says so without costing
	// the person a fresh proof; asked again under the lock below, which is the answer that counts.
	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, err := h.openable(ctx, actor, cmd.HubID, opening.OpenedAt)
		return err
	}); err != nil {
		return domain.EmergencyAccess{}, err
	}
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
		return domain.EmergencyAccess{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		members, err := h.openable(ctx, actor, cmd.HubID, opening.OpenedAt)
		if err != nil {
			return err
		}
		if err := h.Accesses.Insert(ctx, opening); err != nil {
			return err
		}
		// The members are told in this transaction, before it commits: the access counts only once
		// it is committed, and then their records are there with it - or neither is.
		if err := h.Notices.EmergencyAccessOpened(ctx, opening, members); err != nil {
			return err
		}
		return h.Audit.Append(ctx, h.entry(ctx, actor, opening))
	})
	if err != nil {
		return domain.EmergencyAccess{}, err
	}
	return opening, nil
}

// authorize asks for the workspace's owner, before any transaction so that a refusal is recorded.
//
// DELETE_CONTAINER on the workspace alone is the owner's: it is the one permission only OWNER
// carries, and a path of the workspace alone leaves every role held on a hub out of the answer.
// Asked before the hub is looked up, so that nobody but the owner learns whether an identifier
// names a private hub.
func (h OpenPrivateHub) authorize(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) error {
	request := access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     EmergencyAccessOpenedAction,
		TokenScope: containersWrite,
		TargetType: containerTarget,
		TargetID:   hubID,
	}
	if actor.Kind != appshared.ActorUser {
		// A person opens a private hub and is told why they may; a service account or a rule
		// acting for somebody has nobody to give the reason or the proof.
		if err := actor.RequireScope(containersWrite); err != nil {
			return err
		}
		return shared.ErrForbidden.WithDetail("access.not_permitted").
			WithParams(map[string]string{"permission": string(service.PermissionDeleteContainer)})
	}
	return h.Authorizer.Authorize(ctx, actor, request)
}

// openable takes the hub's row and answers its members, or why it cannot be opened now.
func (h OpenPrivateHub) openable(
	ctx context.Context, actor appshared.ActorContext, hubID shared.ID, now time.Time,
) ([]shared.ID, error) {
	hub, found, err := h.Hubs.LockHubOf(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if !found || hub.ID != hubID {
		// Nothing here, or a collection or an entry: only a hub is opened.
		return nil, appshared.ContainerNotFound(hubID)
	}
	if !hub.Private {
		return nil, domain.ErrEmergencyNotPrivate()
	}
	members, err := h.Accesses.Members(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if slices.Contains(members, actor.AccountID) {
		return nil, domain.ErrEmergencyNotNeeded()
	}
	running, open, err := h.Accesses.Running(ctx, hubID, actor.AccountID, now)
	if err != nil {
		return nil, err
	}
	if open {
		return nil, domain.ErrEmergencyRunning(running.EndsAt)
	}
	return members, nil
}

// entry is the trail's record of the opening: the hub, until when, and the reason as "changed"
// with a fingerprint - it is written to the members, often about somebody's health, and the trail
// is read by administrators and auditors who are not them (rule 10, audit.md §4).
func (h OpenPrivateHub) entry(
	ctx context.Context, actor appshared.ActorContext, opening domain.EmergencyAccess,
) audit.Entry {
	return audit.Entry{
		TenantID:   opening.TenantID,
		OccurredAt: opening.OpenedAt,
		Action:     EmergencyAccessOpenedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: containerTarget,
		TargetID:   opening.HubID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "access_id", Classification: audit.Open, To: opening.ID.String()},
			audit.Change{Field: "ends_at", Classification: audit.Open, To: opening.EndsAt.UTC().Format(time.RFC3339)},
			audit.Change{Field: "reason", Classification: audit.Sensitive, To: opening.Reason},
		),
	}
}

// Descriptor is the catalogue entry.
func (h OpenPrivateHub) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: OpenPrivateHubName,
		Summary: "Opens a private hub the workspace's owner holds nothing in, for one hour, through " +
			"the emergency access: a fresh proof and a reason the hub's members read. Every member " +
			"is told at once, and the opening is in the workspace's trail. Not extended; a second " +
			"access while one runs is refused.",
		SideEffects: "Opens the hub to the owner for one hour; notifies every member of the hub by " +
			"mail and in the app; writes an audit entry.",
		TokenScope: containersWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The private hub."},
			{
				// Not Required for the registry: a blank reason is answered by the domain at /reason
				// with its own code, which a person reads, rather than as a malformed input.
				Name: "reason", Kind: usecase.KindString,
				Description: "Why the hub is opened, 1 to 500 characters. Every member reads it.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof opening a private hub demands.",
			},
		},
		StepUp: "opening a private hub through the emergency access",
		Activity: usecase.ActivityDeclaration{
			Exempt: "a container is not an item, and the history is an item's; the hub's own record of " +
				"who opened it is its list of accesses, and the trail's entry",
		},
		Audit: usecase.AuditDeclaration{
			Action: EmergencyAccessOpenedAction, TargetType: containerTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h OpenPrivateHub) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	hubID, err := in.ID("hub_id")
	if err != nil {
		return nil, err
	}
	opening, err := h.Execute(ctx, actor, OpenPrivateHubCommand{
		HubID: hubID, Reason: in.String("reason"), StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return emergencyAccessOutput(opening), nil
}

// emergencyAccessOutput is one access as every answer spells it.
func emergencyAccessOutput(opening domain.EmergencyAccess) usecase.Output {
	var openedBy any
	if !opening.OpenedBy.IsZero() {
		openedBy = opening.OpenedBy.String()
	}
	return usecase.Output{
		"id": opening.ID.String(), "hub_id": opening.HubID.String(), "opened_by": openedBy,
		"reason": opening.Reason, "opened_at": opening.OpenedAt, "ends_at": opening.EndsAt,
	}
}
