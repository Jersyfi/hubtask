// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sealing is the half of a key rotation that lets it finish (ADR-0045, security.md §8.1).
//
// Adding a master key to the ring has always been safe: new values seal under it and every
// predecessor stays readable. Retiring one never was, because nothing rewrapped a value sealed
// under an older key. This package is that rewrap - the operator asks once, one job per workspace
// moves what the workspace holds, and a census over every store says when the older key names
// nothing any more and may leave the ring.
//
// The values themselves stay where they are and with whom they are. Five application services
// own a sealed value each - the second factor, the identity provider's client secret, a webhook's
// signing secrets, a backup target's credential, a rule's HTTP header secret - and each is bound
// to a purpose only its owner can name. So each owner re-seals its own rows behind Resealer, and
// this package composes them without learning a single purpose.
package sealing

import (
	"context"
	"errors"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	// adminTenantsScope is the control plane's credential (0.6.0 decision 6): a personal access
	// token minted for it behind a step-up, never a session. Re-sealing is the control plane's
	// act - it reaches every workspace - so it asks for the same scope.
	adminTenantsScope = "admin:tenants"
	tenantTarget      = "tenant"

	// ResealRequestedAction is the operator's ask, written into the operator's own trail: which
	// key the work will seal under and how many workspaces were given a job.
	ResealRequestedAction audit.Action = "encryption.reseal_requested"
	// ResealedAction is one workspace's round, written into that workspace's trail (audit.md §6):
	// its members are entitled to know the system rewrapped their credentials, even though not
	// one of them changed.
	ResealedAction audit.Action = "encryption.resealed"

	outcomeRewrapped = "rewrapped"
	outcomeSkipped   = "skipped"
)

// Outcome is what one pass over one store did.
type Outcome struct {
	// Rewrapped values now name the current key.
	Rewrapped int64
	// Skipped values name a key the ring no longer holds. They cannot be moved until the
	// operator puts the key back, which is what the census shows them as.
	Skipped int64
}

func (o Outcome) add(other Outcome) Outcome {
	return Outcome{Rewrapped: o.Rewrapped + other.Rewrapped, Skipped: o.Skipped + other.Skipped}
}

// Resealer re-seals one store's rows of the current tenant. Implemented by the service that owns
// the store, because the purpose a value is bound to is that service's alone.
type Resealer interface {
	// Store labels the outcome metric. A closed set written by hand, one per place a sealed value
	// lives (observability-reliability.md §3.2).
	Store() string

	// Reseal moves every value of the transaction's tenant that names a key other than the
	// current one, and reports what it moved and what it had to leave. The tenant is passed for
	// the one store whose purpose is bound to the workspace rather than to a row.
	Reseal(ctx context.Context, tenantID shared.ID) (Outcome, error)
}

// Signals is where a round reports, per store and outcome.
type Signals interface {
	SecretResealed(ctx context.Context, store, outcome string, count int64)
}

// Unopenable reports the refusal a resealer may skip over rather than fail on: a value sealed
// under a key the ring no longer holds. Everything else - a changed byte, a foreign purpose, a
// store that cannot be reached - is an error the job should fail with, because it is not a
// state the operator repairs by configuration.
func Unopenable(err error) bool {
	return errors.Is(err, shared.ErrUnavailable) &&
		shared.AsError(err).DetailCode == cryptoport.CodeUnknownKey
}

// RunReseal is one workspace's round: every resealer in turn, inside the transaction the queue
// opened for the tenant, and one audit entry when anything moved.
type RunReseal struct {
	Resealers []Resealer
	Encryptor cryptoport.Encryptor
	Audit     audit.Sink
	Clock     clock.Clock
	Signals   Signals
}

// Execute runs the round for the actor's tenant and answers the totals.
func (r RunReseal) Execute(ctx context.Context, actor appshared.ActorContext) (Outcome, error) {
	active := r.Encryptor.ActiveKeyID()
	if active == "" {
		// An installation that encrypts nothing has nothing to move. A refusal rather than a
		// silent zero, because a job for it means the operator believed otherwise.
		return Outcome{}, shared.ErrUnavailable.WithDetail(cryptoport.CodeNoEncryptionKey)
	}

	var total Outcome
	for _, resealer := range r.Resealers {
		outcome, err := resealer.Reseal(ctx, actor.TenantID)
		if err != nil {
			return total, err
		}
		if r.Signals != nil {
			r.Signals.SecretResealed(ctx, resealer.Store(), outcomeRewrapped, outcome.Rewrapped)
			r.Signals.SecretResealed(ctx, resealer.Store(), outcomeSkipped, outcome.Skipped)
		}
		total = total.add(outcome)
	}

	if total.Rewrapped == 0 {
		// Nothing moved, nothing to record: a round that found every value already under the
		// current key is the normal second run, and an entry per retry would put noise where
		// evidence lives.
		return total, nil
	}
	return total, r.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: r.Clock.Now(), Action: ResealedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID,
		TargetType: tenantTarget, TargetID: actor.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "active_key_id", Classification: audit.Open, To: active},
			audit.Change{Field: "rewrapped", Classification: audit.Open, To: itoa(total.Rewrapped)},
			audit.Change{Field: "skipped", Classification: audit.Open, To: itoa(total.Skipped)},
		),
	})
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 20)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
