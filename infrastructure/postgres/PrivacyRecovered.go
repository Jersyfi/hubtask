// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.RecoveredCases = PrivacyRepository{}

// InsertAsItWas writes an erasure case a point-in-time recovery rewound away, open, as it stood at
// the end of the archive.
func (r PrivacyRepository) InsertAsItWas(ctx context.Context, request domain.Request) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(request.ID)
	if err != nil {
		return false, err
	}
	subject, err := optionalUUID(request.SubjectAccountID)
	if err != nil {
		return false, err
	}
	handler, err := optionalUUID(request.HandledBy)
	if err != nil {
		return false, err
	}
	rows, err := queries.InsertRecoveredErasureCase(ctx, sqlc.InsertRecoveredErasureCaseParams{
		ID: id, SubjectAccountID: subject, SubjectEmail: optionalText(request.SubjectEmail),
		ErasureMode: optionalText(string(request.ErasureMode)),
		ReceivedAt:  timestampOf(request.ReceivedAt), DueAt: timestampOf(request.DueAt),
		HandledBy: handler, Notes: optionalText(request.Notes),
		OriginalDueAt:   optionalInstant(request.OriginalDueAt),
		ExtensionReason: optionalText(string(request.ExtensionReason)),
		InformedOn:      dateOf(request.InformedOn),
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a rewound erasure case again: %w", err))
	}
	return rows > 0, nil
}

// CompleteIfOpen completes a case at its first completion's moment while it is still open.
func (r PrivacyRepository) CompleteIfOpen(
	ctx context.Context, id shared.ID, mode domain.ErasureMode, completedAt time.Time, subject shared.ID,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	account, err := optionalUUID(subject)
	if err != nil {
		return false, err
	}
	rows, err := queries.CompleteReappliedErasure(ctx, sqlc.CompleteReappliedErasureParams{
		ID: key, ErasureMode: optionalText(string(mode)), CompletedAt: timestampOf(completedAt),
		SubjectAccountID: account,
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("completing a re-applied erasure: %w", err))
	}
	return rows > 0, nil
}

// SubjectExists answers whether the workspace has the account.
func (r PrivacyRepository) SubjectExists(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	present, err := queries.SubjectAccountExists(ctx, key)
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("looking for an erasure's subject: %w", err))
	}
	return present, nil
}
