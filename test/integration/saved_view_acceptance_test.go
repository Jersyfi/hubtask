// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The security content of D-07, against real memberships and real rows: a view shared into a
// scope executes under the *reader's* authorisation. There is no execute endpoint - a client
// reads the view and runs the query as itself - so the proof is equality: per role, the stored
// query answers exactly what the same query asked ad hoc answers, and nobody's answer widens
// because the query arrived through a bookmark somebody else saved (T-04, T-05).

type viewAcceptance struct {
	create work.CreateSavedView
	get    work.GetSavedView
	query  work.QueryItems
}

func viewAcceptanceHarness(ctx context.Context, t *testing.T) viewAcceptance {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork,
		Audit:       postgres.NewAuditSink(ids),
		Clock:       fixed,
	}

	return viewAcceptance{
		create: work.CreateSavedView{
			Views: postgres.NewSavedViewRepository(), Containers: containerRepo(),
			Authorizer: authorizer, Audit: postgres.NewAuditSink(ids),
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
		},
		get: work.GetSavedView{
			Views: postgres.NewSavedViewRepository(), Containers: containerRepo(),
			Permits: authorizer, UnitOfWork: unitOfWork,
		},
		query: work.QueryItems{
			Items: itemRepo(), Containers: containerRepo(), Authorizer: authorizer,
			UnitOfWork: unitOfWork, Clock: fixed,
		},
	}
}

func viewReader(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: account,
		AccountName: "Reader", Scopes: []string{"items:read", "containers:read", "containers:write"},
		TimeZone: "Europe/Berlin",
	}
}

func TestASharedViewExecutesUnderTheReadersAuthorisation(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	harness := viewAcceptanceHarness(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)

	// Two entries, one due soon: the stored filter answers exactly one of them.
	dueSoon := findWorkItem(ctx, t, tenantA, seedTask(ctx, t, tenantA, authorA, collection))
	dueSoon.Due = &domain.DueDate{At: created.Add(24 * time.Hour)}
	dueSoon.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, dueSoon, dueSoon.Version)
	}); err != nil {
		t.Fatalf("seeding the due date: %v", err)
	}
	seedTask(ctx, t, tenantA, authorA, collection)

	// The owner - an admin - saves and shares the view.
	stored, err := harness.create.Execute(ctx, viewReader(authorA), work.CreateSavedViewCommand{
		ScopeType: view.ViewScopeCollection, ScopeID: collection,
		Name: "Due this week", Layout: "KANBAN",
		Query: map[string]any{
			"scope":  map[string]any{"container_id": collection.String()},
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P3D"},
		},
		Sharing: view.SharingScope,
	})
	if err != nil {
		t.Fatalf("saving the shared view failed: %v", err)
	}

	// A viewer on the collection, and a stranger with no membership at all.
	viewer := seedAccount(ctx, t, tenantA)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'COLLECTION', $4, 'VIEWER')`,
		freshID(t).String(), tenantA.String(), viewer.String(), collection.String()); err != nil {
		t.Fatalf("seeding the viewer: %v", err)
	}
	stranger := seedAccount(ctx, t, tenantA)

	// executes runs one query document as one reader, through the same use case a client's
	// items:query reaches.
	executes := func(reader appshared.ActorContext, document map[string]any) ([]string, error) {
		in := usecase.Input{"scope_container_id": collection.String()}
		if filter, has := document["filter"]; has {
			in["filter"] = filter
		}
		out, err := harness.query.Descriptor().Handler.Invoke(ctx, reader, in)
		if err != nil {
			return nil, err
		}
		rows, _ := out["data"].([]usecase.Output)
		titles := make([]string, 0, len(rows))
		for _, row := range rows {
			titles = append(titles, row.String("title"))
		}
		return titles, nil
	}

	t.Run("a viewer reads the view and its query answers their own reach", func(t *testing.T) {
		found, err := harness.get.Execute(ctx, viewReader(viewer), stored.ID)
		if err != nil {
			t.Fatalf("the viewer cannot read the shared view: %v", err)
		}

		fromView, err := executes(viewReader(viewer), found.Query)
		if err != nil {
			t.Fatalf("the stored query failed for the viewer: %v", err)
		}
		adHoc, err := executes(viewReader(viewer), map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P3D"},
		})
		if err != nil {
			t.Fatalf("the ad-hoc query failed for the viewer: %v", err)
		}
		if len(fromView) != 1 || len(adHoc) != 1 || fromView[0] != adHoc[0] {
			t.Errorf("the stored query answered %v, the ad-hoc one %v", fromView, adHoc)
		}
	})

	t.Run("a stranger is answered alike by the view and by their own query", func(t *testing.T) {
		// The view itself is invisible: shared into a scope the stranger holds nothing on, it
		// answers exactly what a missing view answers (T-04).
		if _, err := harness.get.Execute(ctx, viewReader(stranger), stored.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("the stranger's read answered %v", err)
		}

		// And had the document leaked to them, executing it buys nothing their own query would
		// not: the same refusal, because only the reader's own authorisation decides.
		_, fromView := executes(viewReader(stranger), stored.Query)
		_, adHoc := executes(viewReader(stranger), map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P3D"},
		})
		if fromView == nil || adHoc == nil {
			t.Fatal("a stranger's query succeeded")
		}
		// Equality is the acceptance: the same status and the same code, whichever refusal the
		// query endpoint gives a stranger - the bookmark added nothing.
		if shared.AsError(fromView).DetailCode != shared.AsError(adHoc).DetailCode {
			t.Errorf("the refusals differ: %v and %v", fromView, adHoc)
		}
	})

	t.Run("the owner answers their own reach through the same document", func(t *testing.T) {
		fromView, err := executes(viewReader(authorA), stored.Query)
		if err != nil {
			t.Fatalf("the stored query failed for the owner: %v", err)
		}
		if len(fromView) != 1 {
			t.Errorf("the owner's answer is %v", fromView)
		}
	})
}
