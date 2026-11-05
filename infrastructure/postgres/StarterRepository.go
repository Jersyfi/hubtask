// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// StarterRepository keeps whether the workspace's first start was answered (db/queries/Starter.sql).
type StarterRepository struct{}

func NewStarterRepository() StarterRepository { return StarterRepository{} }

var _ repository.Starter = StarterRepository{}

// Pending reports whether the question is still to be asked.
func (StarterRepository) Pending(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	pending, err := queries.StarterPending(ctx)
	if err != nil {
		if IsNoRows(err) {
			return false, nil
		}
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the first start: %w", err))
	}
	return pending, nil
}

// Close records the answer, and reports whether this call recorded it.
func (StarterRepository) Close(ctx context.Context, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	closed, err := queries.CloseStarter(ctx, timestampOf(at))
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("closing the first start: %w", err))
	}
	return closed == 1, nil
}

// Skipped reports whether the question was skipped and the workspace is still as it was created.
func (StarterRepository) Skipped(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	skipped, err := queries.StarterSkipped(ctx)
	if err != nil {
		if IsNoRows(err) {
			return false, nil
		}
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a skipped first start: %w", err))
	}
	return skipped, nil
}

// Hold locks the tenant row until the transaction ends.
func (StarterRepository) Hold(ctx context.Context) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.HoldStarter(ctx); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("holding the first start: %w", err))
	}
	return nil
}
