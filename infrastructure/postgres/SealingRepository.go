// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SealingRepository is the census a rotation ends on (ADR-0045): one statement over the five
// places a sealed value lives, bounded to the transaction's tenant by row level security like
// every other statement here.
type SealingRepository struct{}

func NewSealingRepository() SealingRepository { return SealingRepository{} }

var _ repository.Census = SealingRepository{}

func (SealingRepository) CountByKey(ctx context.Context) (map[string]int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.CountSealedValuesByKey(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the sealed values by key: %w", err))
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.KeyID] = row.SealedValues
	}
	return counts, nil
}
