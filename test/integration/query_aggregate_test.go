// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

func aggregated(
	ctx context.Context, t *testing.T, tenant shared.ID, search repository.ItemSearch, raw map[string]any,
) repository.ItemAggregateResult {
	t.Helper()
	aggregation, err := view.ParseAggregation(raw, "/aggregate")
	if err != nil {
		t.Fatal(err)
	}
	aggregation, err = aggregation.Shaped(nil)
	if err != nil {
		t.Fatal(err)
	}
	var result repository.ItemAggregateResult
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		result, err = itemRepo().Aggregate(ctx, search, aggregation, "Europe/Berlin")
		return err
	}); err != nil {
		t.Fatalf("aggregating: %v", err)
	}
	return result
}

// An aggregation counts what the query keeps, per group and per period, and never another
// tenant's entries (S-11).
func TestAnAggregationCountsWhatTheQueryKeeps(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	seedTask(ctx, t, tenantA, authorA, collection)
	seedTask(ctx, t, tenantA, authorA, collection)

	search := searchIn(t, collection, nil, view.Spec{})
	result := aggregated(ctx, t, tenantA, search, map[string]any{
		"function": "COUNT", "group_by": "is_completed",
		"bucket": map[string]any{"field": "created_at", "period": "day"},
	})
	if len(result.Rows) != 1 || result.Rows[0].Count != 2 || result.Rows[0].Group != "false" ||
		result.Rows[0].Value != nil || result.Truncated {
		t.Fatalf("the aggregation answered %+v, want two open entries on one day", result)
	}
	day, err := time.Parse(time.DateOnly, result.Rows[0].Period)
	if err != nil {
		t.Fatalf("the period is not a day: %q", result.Rows[0].Period)
	}

	monthly := aggregated(ctx, t, tenantA, search, map[string]any{
		"function": "COUNT", "bucket": map[string]any{"field": "created_at", "period": "month"},
	})
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	if len(monthly.Rows) != 1 || monthly.Rows[0].Period != first || monthly.Rows[0].Group != "" {
		t.Errorf("the month answered %+v, want %s", monthly, first)
	}

	other := aggregated(ctx, t, tenantB, search, map[string]any{"function": "COUNT"})
	if len(other.Rows) != 1 || other.Rows[0].Count != 0 {
		t.Errorf("another tenant counted %+v", other)
	}
}
