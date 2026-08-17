// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package audit writes the evidence trail and keeps its chain (audit.md §3, ADR-0017).
package audit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

// Link is what an entry adds to the chain: its position and its hash.
type Link struct {
	Seq  int64
	Hash []byte
}

// Chain computes the hash of an entry, given the hash of the one before it.
//
// `hash = SHA-256(prev_hash ‖ canonical serialisation)`, one chain per tenant (audit.md §3). Its
// limit is deliberate and worth stating: it proves tampering *inside* the database, not against
// an attacker with full database access who recomputes every hash afterwards. Anyone who needs
// that exports the daily chain end to an append-only target - which is a job, not a stronger hash.
func Chain(previousHash []byte, id shared.ID, seq int64, entry port.Entry) ([]byte, error) {
	canonical, err := Canonical(id, seq, entry)
	if err != nil {
		return nil, err
	}

	digest := sha256.New()
	digest.Write(previousHash)
	digest.Write(canonical)
	return digest.Sum(nil), nil
}

// canonicalEntry is the serialisation the hash is taken over.
//
// A struct rather than the entry itself, for two reasons. The field order is fixed by the
// declaration, so two runs of the same version produce the same bytes; and every field is named
// here, so adding a field to the entry cannot silently change what is covered by the hash - it
// has to be added here too, deliberately, which is the moment to think about the entries already
// chained.
type canonicalEntry struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Seq         int64          `json:"seq"`
	OccurredAt  string         `json:"occurred_at"`
	Action      string         `json:"action"`
	Outcome     string         `json:"outcome"`
	Severity    string         `json:"severity"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id"`
	ActorLabel  string         `json:"actor_label"`
	OnBehalfOf  string         `json:"on_behalf_of"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	TargetLabel string         `json:"target_label"`
	Context     port.Context   `json:"context"`
	Changes     map[string]any `json:"changes"`
	LegalBasis  string         `json:"legal_basis"`
}

// Canonical is the byte form the hash covers. Exported because verification has to be able to
// recompute it from a stored row (`POST /audit:verify`), and a verifier that used a second
// implementation would prove the two implementations agree rather than that the chain is intact.
func Canonical(id shared.ID, seq int64, entry port.Entry) ([]byte, error) {
	canonical, err := json.Marshal(canonicalEntry{
		ID:       id.String(),
		TenantID: entry.TenantID.String(),
		Seq:      seq,
		// UTC and nanoseconds: a timestamp rendered in a local zone would hash differently after
		// a server moved, and the column keeps microseconds.
		OccurredAt:  entry.OccurredAt.UTC().Format(time.RFC3339Nano),
		Action:      string(entry.Action),
		Outcome:     string(entry.Outcome),
		Severity:    string(entry.Severity),
		ActorType:   string(entry.ActorKind),
		ActorID:     entry.ActorID.String(),
		ActorLabel:  entry.ActorLabel,
		OnBehalfOf:  entry.OnBehalfOf.String(),
		TargetType:  entry.TargetType,
		TargetID:    entry.TargetID.String(),
		TargetLabel: entry.TargetLabel,
		Context:     entry.Context,
		// Marshalled as part of this struct rather than separately: encoding/json sorts map keys,
		// so the same changes always produce the same bytes.
		Changes:    entry.Changes,
		LegalBasis: entry.LegalBasis,
	})
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("audit.entry_unserialisable").
			WithCause(fmt.Errorf("serialising the audit entry: %w", err))
	}
	return canonical, nil
}
