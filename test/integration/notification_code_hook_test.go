// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A notice about a forge hook that switched itself off, against a real database: the row carries the
// hook as its subject through migration 0353's foreign key, another workspace cannot read it, and
// the hook's removal removes the notice with it.
func TestASwitchedOffHookNoticeNamesItsHookAndGoesWithIt(t *testing.T) {
	ctx := context.Background()
	hub, _ := seedCodeEntry(ctx, t, tenantA)
	recipient := seedAccount(ctx, t, tenantA)
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	hook := forge.Hook{ID: freshID(t), HubID: hub, Kind: forge.GitLab, Name: "web", Enabled: false,
		Salt: "0123456789abcdef0123", RunAs: authorA, CreatedAt: at, UpdatedAt: at}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewCodeForgeRepository().PutHook(ctx, hook)
	}); err != nil {
		t.Fatalf("storing the hook: %v", err)
	}

	record, err := domain.New(domain.NewInput{
		ID: freshID(t), TenantID: tenantA, RecipientID: recipient,
		Category: domain.CategoryIntegration, Channel: domain.ChannelEmail,
		CodeHookID: hook.ID, At: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !insertNotification(ctx, t, tenantA, record) {
		t.Fatal("the first write reported an existing record")
	}
	stored, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil || stored.CodeHookID != hook.ID {
		t.Fatalf("read back hook %v, %v; want %v", stored.CodeHookID, err, hook.ID)
	}
	if _, err := findNotification(ctx, t, tenantB, record.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B read A's notice: %v", err)
	}

	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM code_hook WHERE id = $1`, hook.ID.String()); err != nil {
		t.Fatalf("removing the hook: %v", err)
	}
	if _, err := findNotification(ctx, t, tenantA, record.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the notice outlived its hook: %v", err)
	}
}
