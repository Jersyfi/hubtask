// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SampleEntryRepository stores the marks of a template's sample entries (migration 0314).
type SampleEntryRepository struct{}

var _ repository.Samples = SampleEntryRepository{}

func sampleFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Mark marks the entry; a key the hub marks already writes nothing.
func (SampleEntryRepository) Mark(ctx context.Context, hub, item shared.ID, key string, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	hubKey, err := uuidOf(hub)
	if err != nil {
		return false, err
	}
	itemKey, err := uuidOf(item)
	if err != nil {
		return false, err
	}
	n, err := queries.InsertSampleEntry(ctx, sqlc.InsertSampleEntryParams{
		ItemID: itemKey, HubID: hubKey, Key: key, CreatedAt: timestampOf(at),
	})
	if err != nil {
		return false, sampleFailed("marking a sample entry", err)
	}
	return n == 1, nil
}

// List answers the hub's live sample entries.
func (SampleEntryRepository) List(ctx context.Context, hub shared.ID) ([]repository.Sample, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hubKey, err := uuidOf(hub)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSampleEntries(ctx, hubKey)
	if err != nil {
		return nil, sampleFailed("listing sample entries", err)
	}
	out := make([]repository.Sample, 0, len(rows))
	for _, row := range rows {
		item, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		sample := repository.Sample{Key: row.Key, ItemID: item, Title: row.Title, CollectionID: collection}
		if row.EntryType != nil {
			sample.EntryType = *row.EntryType
		}
		out = append(out, sample)
	}
	return out, nil
}
