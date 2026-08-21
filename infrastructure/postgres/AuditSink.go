// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/audit"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// AuditSink appends to the trail, inside the caller's transaction.
//
// The application role may only INSERT and SELECT on audit_log, a trigger refuses an UPDATE or a
// DELETE, and the hash chain makes a rewrite detectable - three levels, because one is not enough
// (audit.md §3). This type is responsible for the third; the chain itself is computed in
// infrastructure/audit, which needs no database in order to be tested.
//
// project-structure.md sketches this file as infrastructure/audit/PostgresAuditSink.go. It lives
// here instead, because rule 3 keeps the driver inside this package: a sink elsewhere would
// either hold pgx itself - which the architecture test refuses - or need a second set of type
// conversions exported to it.
type AuditSink struct {
	IDs clock.IDGenerator
}

func NewAuditSink(ids clock.IDGenerator) AuditSink { return AuditSink{IDs: ids} }

var _ port.Sink = AuditSink{}

// Append writes one entry and extends the tenant's chain.
//
// The chain is serialised per tenant with a transaction-scoped advisory lock. Two transactions
// writing for the same tenant would otherwise read the same tail and produce two entries with the
// same sequence number and the same predecessor - a chain that forks, which is indistinguishable
// from one that was tampered with. Per tenant rather than globally, so one busy workspace does
// not serialise the installation; and transaction-scoped, so a process that dies releases it.
func (s AuditSink) Append(ctx context.Context, entry port.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	tx, err := FromContext(ctx)
	if err != nil {
		return err
	}
	// The entry has to belong to the tenant the transaction runs as. The insert takes the tenant
	// from current_tenant_id() either way, but the hash covers the entry's own value - and a hash
	// over a tenant the row does not have would break verification later, silently.
	if scope, ok := scopeFromContext(ctx); ok && scope.TenantID != entry.TenantID {
		return shared.ErrInternal.
			WithDetail("audit.tenant_mismatch").
			WithParams(map[string]string{"entry": entry.TenantID.String(), "transaction": scope.TenantID.String()})
	}
	queries := sqlc.New(tx)

	if err := queries.LockAuditChain(ctx); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("locking the audit chain: %w", err))
	}

	var previous audit.Link
	tail, err := queries.LastAuditEntry(ctx)
	switch {
	case err == nil:
		previous = audit.Link{Seq: tail.Seq, Hash: tail.Hash}
	case IsNoRows(err):
		// The first entry of this tenant. Its previous hash stays NULL, which is what makes the
		// start of a chain recognisable rather than indistinguishable from a truncated one.
	default:
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the audit chain: %w", err))
	}

	id := s.IDs.NewID()
	seq := previous.Seq + 1
	hash, err := audit.Chain(previous.Hash, id, seq, entry)
	if err != nil {
		return err
	}

	params, err := auditInsertParams(id, seq, hash, previous.Hash, entry)
	if err != nil {
		return err
	}
	if err := queries.InsertAuditEntry(ctx, params); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the audit entry: %w", err))
	}
	return nil
}

func auditInsertParams(id shared.ID, seq int64, hash, previousHash []byte, entry port.Entry) (sqlc.InsertAuditEntryParams, error) {
	var params sqlc.InsertAuditEntryParams

	entryID, err := uuidOf(id)
	if err != nil {
		return params, err
	}
	actorID, err := optionalUUID(entry.ActorID)
	if err != nil {
		return params, err
	}
	onBehalfOf, err := optionalUUID(entry.OnBehalfOf)
	if err != nil {
		return params, err
	}
	targetID, err := optionalUUID(entry.TargetID)
	if err != nil {
		return params, err
	}

	auditContext, err := json.Marshal(entry.Context)
	if err != nil {
		return params, shared.ErrInternal.
			WithDetail("audit.entry_unserialisable").
			WithCause(fmt.Errorf("serialising the audit context: %w", err))
	}
	changes := entry.Changes
	if changes == nil {
		// The column is NOT NULL with an empty object as its default; a nil map marshals to
		// `null`, which the column would refuse.
		changes = map[string]any{}
	}
	changed, err := json.Marshal(changes)
	if err != nil {
		return params, shared.ErrInternal.
			WithDetail("audit.entry_unserialisable").
			WithCause(fmt.Errorf("serialising the audit changes: %w", err))
	}

	return sqlc.InsertAuditEntryParams{
		ID:           entryID,
		Seq:          seq,
		OccurredAt:   timestampOf(entry.OccurredAt),
		Action:       string(entry.Action),
		Outcome:      string(entry.Outcome),
		Severity:     string(entry.Severity),
		ActorType:    string(entry.ActorKind),
		ActorID:      actorID,
		ActorLabel:   optionalText(entry.ActorLabel),
		OnBehalfOfID: onBehalfOf,
		TargetType:   optionalText(entry.TargetType),
		TargetID:     targetID,
		TargetLabel:  optionalText(entry.TargetLabel),
		Context:      auditContext,
		Changes:      changed,
		LegalBasis:   optionalText(entry.LegalBasis),
		PrevHash:     previousHash,
		Hash:         hash,
	}, nil
}
