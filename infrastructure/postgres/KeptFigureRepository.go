// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// KeptFigureRepository keeps collections' counts between reads (db/queries/KeptFigures.sql,
// migration 0292). A kept row whose counts or basis do not decode is answered as not kept: it is
// counted again rather than trusted.
type KeptFigureRepository struct{}

func NewKeptFigureRepository() KeptFigureRepository { return KeptFigureRepository{} }

var _ repository.KeptFigures = KeptFigureRepository{}

// keptCounts is the stored shape of work.FigureCounts; the names are the store's, so a renamed
// domain field never reads an old row as zero.
type keptCounts struct {
	Open           int      `json:"open"`
	Sum            *string  `json:"sum"`
	Summed         int      `json:"summed"`
	Due            int      `json:"due"`
	Done           int      `json:"done"`
	Total          int      `json:"total"`
	Days           []string `json:"days"`
	Completed      int      `json:"completed"`
	AverageSeconds *float64 `json:"average_seconds"`
	OverLimit      int      `json:"over_limit"`
}

// keptBasis is the stored shape of work.FigureBasis.
type keptBasis struct {
	Offered   []string  `json:"offered"`
	Dated     []string  `json:"dated"`
	FieldKey  string    `json:"field_key"`
	Today     string    `json:"today"`
	DayEnd    time.Time `json:"day_end"`
	Zone      string    `json:"zone"`
	WeekStart time.Time `json:"week_start"`
}

func encodeCounts(c work.FigureCounts) ([]byte, error) {
	return json.Marshal(keptCounts{
		Open: c.Open, Sum: c.Sum, Summed: c.Summed, Due: c.Due, Done: c.Done, Total: c.Total,
		Days: c.Days, Completed: c.Completed, AverageSeconds: c.AverageSeconds, OverLimit: c.OverLimit,
	})
}

func decodeCounts(raw []byte) (work.FigureCounts, error) {
	var k keptCounts
	if err := json.Unmarshal(raw, &k); err != nil {
		return work.FigureCounts{}, err
	}
	return work.FigureCounts{
		Open: k.Open, Sum: k.Sum, Summed: k.Summed, Due: k.Due, Done: k.Done, Total: k.Total,
		Days: k.Days, Completed: k.Completed, AverageSeconds: k.AverageSeconds, OverLimit: k.OverLimit,
	}, nil
}

func typeNames(types []work.ItemType) []string {
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, string(t))
	}
	return names
}

func itemTypes(names []string) []work.ItemType {
	types := make([]work.ItemType, 0, len(names))
	for _, n := range names {
		types = append(types, work.ItemType(n))
	}
	return types
}

func encodeBasis(b work.FigureBasis) ([]byte, error) {
	return json.Marshal(keptBasis{
		Offered: typeNames(b.Offered), Dated: typeNames(b.Dated), FieldKey: b.FieldKey,
		Today: b.Today, DayEnd: b.DayEnd, Zone: b.Zone, WeekStart: b.WeekStart,
	})
}

func decodeBasis(raw []byte) (work.FigureBasis, error) {
	var k keptBasis
	if err := json.Unmarshal(raw, &k); err != nil {
		return work.FigureBasis{}, err
	}
	return work.FigureBasis{
		Offered: itemTypes(k.Offered), Dated: itemTypes(k.Dated), FieldKey: k.FieldKey,
		Today: k.Today, DayEnd: k.DayEnd, Zone: k.Zone, WeekStart: k.WeekStart,
	}, nil
}

func keptFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s kept figures: %w", what, err))
}

// States answers each wanted figure's number of changes and what is kept, in one statement.
func (KeptFigureRepository) States(
	ctx context.Context, wanted []repository.KeptFigureKey,
) (map[repository.KeptFigureKey]repository.KeptFigureState, error) {
	states := make(map[repository.KeptFigureKey]repository.KeptFigureState, len(wanted))
	if len(wanted) == 0 {
		return states, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.KeptFiguresParams{}
	for _, key := range wanted {
		id, err := uuidOf(key.CollectionID)
		if err != nil {
			return nil, err
		}
		params.CollectionIds = append(params.CollectionIds, id)
		params.Kinds = append(params.Kinds, string(key.Kind))
		params.BasisKeys = append(params.BasisKeys, key.BasisKey)
	}
	rows, err := queries.KeptFigures(ctx, params)
	if err != nil {
		return nil, keptFailed("reading", err)
	}
	for _, row := range rows {
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		key := repository.KeptFigureKey{CollectionID: collectionID, Kind: work.Figure(row.Kind), BasisKey: row.BasisKey}
		state := repository.KeptFigureState{Changes: row.Changes}
		if row.KeptChanges != nil {
			if counts, err := decodeCounts(row.Counts); err == nil {
				state.Kept = repository.KeptFigure{
					Key: key, Changes: *row.KeptChanges, Counts: counts,
					CountedAt: timeFrom(row.CountedAt), Present: true,
				}
			}
		}
		states[key] = state
	}
	return states, nil
}

// Keep stores the counts, one statement each: only the counts a read had to make.
func (KeptFigureRepository) Keep(ctx context.Context, figures []repository.KeptFigure) error {
	if len(figures) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	for _, figure := range figures {
		id, err := uuidOf(figure.Key.CollectionID)
		if err != nil {
			return err
		}
		basis, err := encodeBasis(figure.Basis)
		if err != nil {
			return err
		}
		counts, err := encodeCounts(figure.Counts)
		if err != nil {
			return err
		}
		err = queries.KeepFigure(ctx, sqlc.KeepFigureParams{
			CollectionID: id, Kind: string(figure.Key.Kind), BasisKey: figure.Key.BasisKey,
			Basis: basis, Changes: figure.Changes, Counts: counts, CountedAt: timestampOf(figure.CountedAt),
		})
		if err != nil {
			return keptFailed("keeping", err)
		}
	}
	return nil
}

// Fold moves the recorded changes into the stamps.
func (KeptFigureRepository) Fold(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	folded, err := queries.FoldFigureTouches(ctx)
	if err != nil {
		return 0, keptFailed("folding", err)
	}
	return folded, nil
}

// Prune drops the counts no read can be answered from and the stamps of purged collections.
func (KeptFigureRepository) Prune(ctx context.Context, before time.Time) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	figures, err := queries.PruneKeptFigures(ctx, timestampOf(before))
	if err != nil {
		return 0, keptFailed("pruning", err)
	}
	stamps, err := queries.PruneFigureStamps(ctx)
	if err != nil {
		return 0, keptFailed("pruning the stamps of", err)
	}
	return figures + stamps, nil
}

// ToVerify answers the current kept counts of the kinds, the longest unverified first. A row that
// does not decode is answered with zero counts, so that its recount replaces it.
func (KeptFigureRepository) ToVerify(ctx context.Context, kinds []work.Figure, batch int) ([]repository.KeptFigure, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	rows, err := queries.KeptFiguresToVerify(ctx, sqlc.KeptFiguresToVerifyParams{Kinds: names, Batch: boundedInt32(batch)})
	if err != nil {
		return nil, keptFailed("listing", err)
	}
	out := make([]repository.KeptFigure, 0, len(rows))
	for _, row := range rows {
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		basis, err := decodeBasis(row.Basis)
		if err != nil {
			// A basis nobody can read cannot be recounted; pruning takes it once it ages.
			continue
		}
		counts, _ := decodeCounts(row.Counts)
		out = append(out, repository.KeptFigure{
			Key:   repository.KeptFigureKey{CollectionID: collectionID, Kind: work.Figure(row.Kind), BasisKey: row.BasisKey},
			Basis: basis, Changes: row.Changes, Counts: counts, Present: true,
		})
	}
	return out, nil
}

// Verify writes the recount over the kept count, at the same number of changes only.
func (KeptFigureRepository) Verify(ctx context.Context, figure repository.KeptFigure, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(figure.Key.CollectionID)
	if err != nil {
		return false, err
	}
	counts, err := encodeCounts(figure.Counts)
	if err != nil {
		return false, err
	}
	matched, err := queries.VerifyKeptFigure(ctx, sqlc.VerifyKeptFigureParams{
		Counts: counts, VerifiedAt: timestampOf(at), CollectionID: id,
		Kind: string(figure.Key.Kind), BasisKey: figure.Key.BasisKey, Changes: figure.Changes,
	})
	if err != nil {
		return false, keptFailed("verifying", err)
	}
	return matched == 1, nil
}
