// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ui.md A4: a block's draft is saved under the optimistic lock, removed by the apply that put
// exactly it in force, kept when saved again since, and invisible to the workspace next door
// (migration 0308).
func TestAConfigDraftKeepsItsLockAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	drafts := postgres.ConfigDraftRepository{}
	key := "d" + strings.ReplaceAll(freshID(t).String(), "-", "")[:24]
	at := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	half := domain.Block{"name": "Incident", "form": map[string]any{"fields": []any{map[string]any{"key": "custom:impact", "required": true}}}}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		first, ok, err := drafts.Save(ctx, repository.Draft{Section: "types", Key: key, Block: half, SavedBy: authorA, SavedAt: at}, 0)
		if err != nil || !ok || first.Version != 1 || first.SavedBy != authorA {
			t.Errorf("first save %+v, %v, %v", first, ok, err)
		}
		if _, ok, err := drafts.Save(ctx, repository.Draft{Section: "types", Key: key, Block: half, SavedBy: authorA, SavedAt: at}, 5); err != nil || ok {
			t.Errorf("a save over a moved draft: %v, %v", ok, err)
		}
		second, ok, err := drafts.Save(ctx, repository.Draft{Section: "types", Key: key, Block: domain.Block{"name": "Störung"}, SavedBy: authorA, SavedAt: at}, 1)
		if err != nil || !ok || second.Version != 2 || second.Block["name"] != "Störung" {
			t.Errorf("second save %+v, %v, %v", second, ok, err)
		}
		// An apply of another block keeps the draft; one of exactly the drafted block removes it.
		if err := drafts.Published(ctx, "types", key, half); err != nil {
			t.Error(err)
		}
		if listed, err := drafts.List(ctx); err != nil || !holdsDraft(listed, key) {
			t.Errorf("an apply of another block removed the draft: %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if listed, err := drafts.List(ctx); err != nil || holdsDraft(listed, key) {
			t.Errorf("the workspace next door listed %+v, %v", listed, err)
		}
		if gone, err := drafts.Discard(ctx, "types", key); err != nil || gone {
			t.Errorf("the workspace next door discarded another's draft: %v, %v", gone, err)
		}
		return drafts.Published(ctx, "types", key, domain.Block{"name": "Störung"})
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if listed, _ := drafts.List(ctx); !holdsDraft(listed, key) {
			t.Error("the workspace next door's apply removed the draft")
		}
		if err := drafts.Published(ctx, "types", key, domain.Block{"name": "Störung"}); err != nil {
			return err
		}
		if listed, _ := drafts.List(ctx); holdsDraft(listed, key) {
			t.Error("the apply of the drafted block kept the draft")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func holdsDraft(drafts []repository.Draft, key string) bool {
	for _, d := range drafts {
		if d.Key == key {
			return true
		}
	}
	return false
}
