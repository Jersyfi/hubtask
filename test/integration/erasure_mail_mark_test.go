// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// An anonymised account keeps its row, so the mark of an address the mail server refused - which
// holds that address - went only with a deleted account. The erasure's notification step removes
// it whatever is kept, and only the person's, in their workspace.
func TestAnErasureTakesThePersonsUndeliverableMarkAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	marks := postgres.NewMailUndeliverableRepository()
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return marks.Mark(ctx, authorA, "refused@example.com", at)
	}); err != nil {
		t.Fatalf("marking in A: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return marks.Mark(ctx, authorB, "refused-b@example.com", at)
	}); err != nil {
		t.Fatalf("marking in B: %v", err)
	}

	erasure := postgres.NewPrivacyRepository(pageCursors())
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := erasure.DiscardNotifications(ctx, authorA)
		return err
	}); err != nil {
		t.Fatalf("discarding A's person from B: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if _, _, err := marks.Find(ctx, authorA); err != nil {
			t.Errorf("B's erasure reached A's mark: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := erasure.DiscardNotifications(ctx, authorA)
		return err
	}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if _, _, err := marks.Find(ctx, authorA); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the erased person's refused address is still held: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := marks.Find(ctx, authorB); err != nil {
			t.Errorf("another workspace's mark went with the erasure: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
