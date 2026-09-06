// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Enqueuer is the slice of the queue this package uses.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Accepted is what the operator is told: which key the queued work seals under, and how many
// workspaces were given a job.
type Accepted struct {
	ActiveKeyID   string
	QueuedTenants int
}

// ResealSecrets is the operator's ask (ADR-0045): one job per workspace, each enqueued in a
// bounded transaction of that workspace's own, through the control plane's one legitimate
// enumerator. Nothing here is a sweep, and nothing here opens a value.
type ResealSecrets struct {
	Tenants    adminrepo.Tenants
	Jobs       Enqueuer
	Encryptor  cryptoport.Encryptor
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute queues the rounds and records the ask.
func (h ResealSecrets) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Accepted, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return Accepted{}, err
	}
	active := h.Encryptor.ActiveKeyID()
	if active == "" {
		return Accepted{}, shared.ErrUnavailable.WithDetail(cryptoport.CodeNoEncryptionKey)
	}

	var tenants []adminrepo.TenantRecord
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			listed, err := h.Tenants.List(ctx)
			tenants = listed
			return err
		})
	if err != nil {
		return Accepted{}, err
	}

	for _, tenant := range tenants {
		scope := persistence.Scope{TenantID: tenant.ID, ActorID: actor.AccountID}
		err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			_, err := h.Jobs.Enqueue(ctx, queue.Request{
				Kind: queue.KindSecretReseal, TenantID: tenant.ID,
				Payload: map[string]any{"active_key_id": active},
				// Per tenant while pending: asking twice before the first round has run queues
				// nothing new, and a round that has run is done with this key.
				DedupeKey: "reseal:" + tenant.ID.String(),
			})
			return err
		})
		if err != nil {
			return Accepted{}, err
		}
	}

	accepted := Accepted{ActiveKeyID: active, QueuedTenants: len(tenants)}
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(), Action: ResealRequestedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "active_key_id", Classification: audit.Open, To: active},
				audit.Change{Field: "queued_tenants", Classification: audit.Open,
					To: itoa(int64(accepted.QueuedTenants))},
			),
		})
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// Descriptor is the catalogue entry.
func (h ResealSecrets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ResealSecretsName,
		Summary: "Queues, for every workspace of the installation, a round that moves each " +
			"stored value sealed under an older master key under the current one - so that the " +
			"older key can be removed from the ring once nothing names it. Plaintext is never " +
			"reconstructed: only the wrapping of each value's data key changes.",
		SideEffects: "One job per workspace, deduplicated while pending; an audit entry in the " +
			"operator's workspace naming the key and the count, and one in each workspace a " +
			"round moved anything in. Progress is read from the encryption status.",
		TokenScope: adminTenantsScope,
		Audit: usecase.AuditDeclaration{
			Action: ResealRequestedAction, TargetType: tenantTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "an act of the control plane touches no item; the history is an item's " +
				"(domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ResealSecrets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	accepted, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"active_key_id": accepted.ActiveKeyID, "queued_tenants": accepted.QueuedTenants,
	}, nil
}
