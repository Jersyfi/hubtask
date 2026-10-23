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

// Close records the answer.
func (StarterRepository) Close(ctx context.Context, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := queries.CloseStarter(ctx, timestampOf(at)); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("closing the first start: %w", err))
	}
	return nil
}
