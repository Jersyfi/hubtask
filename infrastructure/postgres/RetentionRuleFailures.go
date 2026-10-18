// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.RuleFailures = RetentionRuleRepository{}

// RecordFailure sets the rule's last failure.
func (r RetentionRuleRepository) RecordFailure(
	ctx context.Context, ruleID shared.ID, failure domain.RuleFailure,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(ruleID)
	if err != nil {
		return err
	}
	code := failure.Code
	if _, err := queries.RecordRuleFailure(ctx, sqlc.RecordRuleFailureParams{
		ID: key, Code: &code, At: timestampOf(failure.At),
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording a retention rule's failure: %w", err))
	}
	return nil
}

// ClearFailures takes the last failure off the rules named.
func (r RetentionRuleRepository) ClearFailures(ctx context.Context, ruleIDs []shared.ID) error {
	if len(ruleIDs) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys := make([]pgtype.UUID, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		key, err := uuidOf(id)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	if _, err := queries.ClearRuleFailures(ctx, keys); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing retention rules' failures: %w", err))
	}
	return nil
}
