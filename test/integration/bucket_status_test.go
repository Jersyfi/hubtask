// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Which statuses a board's columns show (migration 0395), against a real database: written,
// read left to right, replaced whole, a status in one column only, a deleted column's rows left
// out, and a cross-tenant negative for every method (gate SG-3).

func bucketStatusRepo() postgres.BucketStatusRepository { return postgres.NewBucketStatusRepository() }

func readMapping(ctx context.Context, t *testing.T, tenant, collection shared.ID) []work.ColumnStatus {
	t.Helper()
	var mapping []work.ColumnStatus
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		mapping, err = bucketStatusRepo().List(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the mapping: %v", err)
	}
	return mapping
}

func TestABoardsMappingIsWrittenReadAndReplaced(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	first := seedBucket(ctx, t, tenantA, collection, "a0")
	second := seedBucket(ctx, t, tenantA, collection, "a1")

	mapping := []work.ColumnStatus{
		{BucketID: second.ID, Status: "review"}, {BucketID: first.ID, Status: "open"},
		{BucketID: second.ID, Status: "doing"},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketStatusRepo().Replace(ctx, collection, mapping)
	}); err != nil {
		t.Fatalf("writing the mapping: %v", err)
	}
	want := []work.ColumnStatus{
		{BucketID: first.ID, Status: "open"}, {BucketID: second.ID, Status: "doing"},
		{BucketID: second.ID, Status: "review"},
	}
	if got := readMapping(ctx, t, tenantA, collection); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{{BucketID: first.ID, Status: "doing"}})
	}); err != nil {
		t.Fatalf("replacing the mapping: %v", err)
	}
	if got := readMapping(ctx, t, tenantA, collection); !slices.Equal(got, []work.ColumnStatus{{BucketID: first.ID, Status: "doing"}}) {
		t.Errorf("the replacement left %v", got)
	}

	t.Run("a status sits in one column only", func(t *testing.T) {
		err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{
				{BucketID: first.ID, Status: "open"}, {BucketID: second.ID, Status: "open"},
			})
		})
		if err == nil {
			t.Fatal("one status went into two columns")
		}
	})

	t.Run("a column of another board is refused", func(t *testing.T) {
		_, other := hubWithCollection(ctx, t, tenantA, authorA)
		foreign := seedBucket(ctx, t, tenantA, other, "a0")
		err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{{BucketID: foreign.ID, Status: "open"}})
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another board's column was mapped: %v", err)
		}
	})

	t.Run("a deleted column's rows are left out", func(t *testing.T) {
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{
				{BucketID: first.ID, Status: "open"}, {BucketID: second.ID, Status: "doing"},
			})
		}); err != nil {
			t.Fatalf("writing the mapping: %v", err)
		}
		gone, _, err := findBucket(ctx, t, tenantA, second.ID).Deleted(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return bucketRepo().SetDeleted(ctx, gone, gone.Version)
		}); err != nil {
			t.Fatalf("deleting the column: %v", err)
		}
		if got := readMapping(ctx, t, tenantA, collection); !slices.Equal(got, []work.ColumnStatus{{BucketID: first.ID, Status: "open"}}) {
			t.Errorf("the deleted column still shows: %v", got)
		}
	})
}

// The cross-tenant negative for both methods (gate SG-3).
func TestABoardsMappingIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	column := seedBucket(ctx, t, tenantA, collection, "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{{BucketID: column.ID, Status: "open"}})
	}); err != nil {
		t.Fatalf("writing the mapping: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		if got := readMapping(ctx, t, tenantB, collection); len(got) != 0 {
			t.Errorf("tenant B read tenant A's mapping: %v", got)
		}
	})

	t.Run("replace", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return bucketStatusRepo().Replace(ctx, collection, []work.ColumnStatus{{BucketID: column.ID, Status: "doing"}})
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B wrote into tenant A's board: %v", err)
		}
		if got := readMapping(ctx, t, tenantA, collection); !slices.Equal(got, []work.ColumnStatus{{BucketID: column.ID, Status: "open"}}) {
			t.Errorf("tenant B's clear reached tenant A: %v", got)
		}
	})
}
