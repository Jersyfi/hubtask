// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SealingRepository is the census a rotation ends on (ADR-0045): one statement over the places a
// sealed value lives, bounded to the transaction's tenant by row level security like every other
// statement here, and one more over the installation's own rows.
type SealingRepository struct{}

func NewSealingRepository() SealingRepository { return SealingRepository{} }

var _ repository.Census = SealingRepository{}

type censusRow struct {
	Store, KeyID string
	SealedValues int64
}

func (SealingRepository) Count(ctx context.Context) (repository.Counts, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.CountSealedValuesByStoreAndKey(ctx)
	if err != nil {
		return nil, censusFailed(err)
	}
	counted := make([]censusRow, 0, len(rows))
	for _, row := range rows {
		counted = append(counted, censusRow(row))
	}
	return countsOf(counted), nil
}

func (SealingRepository) CountInstallation(ctx context.Context) (repository.Counts, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.CountInstallationSealedValuesByStoreAndKey(ctx)
	if err != nil {
		return nil, censusFailed(err)
	}
	counted := make([]censusRow, 0, len(rows))
	for _, row := range rows {
		counted = append(counted, censusRow(row))
	}
	return countsOf(counted), nil
}

func countsOf(rows []censusRow) repository.Counts {
	counts := repository.Counts{}
	for _, row := range rows {
		counts.Add(row.KeyID, row.Store, row.SealedValues)
	}
	return counts
}

func censusFailed(err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("counting the sealed values by key: %w", err))
}
