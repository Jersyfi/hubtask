// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-SVC-13 check 1: a public form is found by its token in its own workspace alone, a new link
// replaces the old, and taking it away leaves nothing to find (migration 0302).
func TestAPublicFormIsFoundByItsTokenInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	forms := postgres.NewPublicFormRepository(security.NewPublicFormHasher(secret.New(feedSecret)))
	at := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	mint := func(tenant shared.ID, fill byte) integration.FormLinkToken {
		token, err := integration.NewFormLinkToken(tenant, bytes.Repeat([]byte{fill}, integration.FormLinkTokenSecretBytes))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	first, second := mint(tenantA, 1), mint(tenantA, 2)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		form := servicedesk.PublicForm{CollectionID: collection, CreatedBy: authorA, CreatedAt: at}
		if err := forms.Put(ctx, form, first); err != nil {
			return err
		}
		if found, err := forms.FindByToken(ctx, first); err != nil || found.CollectionID != collection ||
			found.CreatedBy != authorA || found.TenantID != tenantA || !found.CreatedAt.Equal(at) {
			t.Errorf("by token %+v, %v", found, err)
		}
		if err := forms.Put(ctx, form, second); err != nil {
			return err
		}
		if _, err := forms.FindByToken(ctx, first); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the replaced token still answers: %v", err)
		}
		if _, found, err := forms.Find(ctx, collection); err != nil || !found {
			t.Errorf("find %v, %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := forms.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door found the form: %v", err)
		}
		if _, found, _ := forms.Find(ctx, collection); found {
			t.Error("the workspace next door read the form")
		}
		if removed, _ := forms.Delete(ctx, collection); removed {
			t.Error("the workspace next door removed the form")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if removed, err := forms.Delete(ctx, collection); err != nil || !removed {
			t.Errorf("delete %v, %v", removed, err)
		}
		if _, err := forms.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a removed form still answers: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
