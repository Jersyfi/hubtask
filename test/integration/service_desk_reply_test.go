// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A reply template is stored, replaced by its version and read back; the workspace next door
// neither reads nor deletes it, and a second one under the same key is refused (migration 0242,
// UC-SVC-06).
func TestReplyTemplatesStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	replies := postgres.NewServiceDeskRepository().Replies()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)

	template, err := servicedesk.NewReplyTemplate(servicedesk.NewReplyTemplateInput{
		ID: freshID(t), TenantID: tenantA, Key: "reset", Name: "Password reset",
		Body: "Hello {{requester.name}}", Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return replies.Insert(ctx, template) }); err != nil {
		t.Fatalf("insert: %v", err)
	}
	twin := template
	twin.ID = freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return replies.Insert(ctx, twin) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second template under the key: %v, want reply_templates.key_taken", err)
	}
	changed, err := template.Change("Password reset", "Hi {{requester.name}}", now, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return replies.Update(ctx, changed, 1) }); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return replies.Update(ctx, changed, 1) }); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("an update of an older version: %v", err)
	}

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := replies.List(ctx)
		if err != nil {
			return err
		}
		if len(listed) != 0 {
			t.Errorf("the other workspace lists %d templates", len(listed))
		}
		if _, err := replies.Find(ctx, "reset"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the template: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return replies.Delete(ctx, "reset") }); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deletes the template: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := replies.Find(ctx, "reset")
		if err == nil && (found.Body != "Hi {{requester.name}}" || found.Version != 2) {
			t.Errorf("read back %+v", found)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
