// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CallSealingRepository is the re-seal's reach into the job table (ADR-0045): the HTTP calls still
// waiting to be made or retried, whose header secret lives in the job's payload. Its own type
// rather than a method on the queue's repository, because the queue reads payloads it does not
// understand and this reads exactly one field of one kind.
//
// The job table has no row level security, so the statements name the transaction's workspace
// themselves.
type CallSealingRepository struct{}

func NewCallSealingRepository() CallSealingRepository { return CallSealingRepository{} }

var _ repository.CallSealings = CallSealingRepository{}

func (CallSealingRepository) SealedNotUnder(
	ctx context.Context, keyID string,
) ([]repository.PendingCall, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.CallJobsSealedNotUnder(ctx, keyID)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the pending calls to re-seal: %w", err))
	}
	calls := make([]repository.PendingCall, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		var payload struct {
			RequestID string `json:"request_id"`
			Sealed    struct {
				Ciphertext string `json:"ciphertext"`
				KeyID      string `json:"key_id"`
				Purpose    string `json:"purpose"`
			} `json:"secret_header_sealed"`
		}
		// A payload this cannot read is answered with what could be read: the re-sealer skips a
		// secret it cannot open rather than failing the workspace's round on one job.
		_ = json.Unmarshal(row.Payload, &payload)
		calls = append(calls, repository.PendingCall{
			JobID: id, RequestID: payload.RequestID,
			Sealed: domain.SealedSecret{
				Ciphertext: payload.Sealed.Ciphertext, KeyID: payload.Sealed.KeyID,
				Purpose: payload.Sealed.Purpose,
			},
		})
	}
	return calls, nil
}

func (CallSealingRepository) RewrapCall(
	ctx context.Context, jobID shared.ID, sealed domain.SealedSecret, expectedKeyID string,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(jobID)
	if err != nil {
		return false, err
	}
	document, err := json.Marshal(sealed.Document())
	if err != nil {
		return false, shared.ErrInternal.
			WithDetail("automation.http_job_malformed").
			WithCause(fmt.Errorf("encoding the sealed secret of job %s: %w", jobID, err))
	}
	changed, err := queries.RewrapCallJobSecret(ctx, sqlc.RewrapCallJobSecretParams{
		Sealed: document, ID: key, ExpectedKeyID: expectedKeyID,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rewrapping the secret of job %s: %w", jobID, err))
	}
	return changed == 1, nil
}
