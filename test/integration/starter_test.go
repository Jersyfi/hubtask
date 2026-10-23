// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// The first start's question against PostgreSQL (UC-WRK-30 check 5): offered once to whoever may
// shape a new workspace, closed for good by an answer or a skip, never offered where a hub exists,
// and closed in one workspace only. Each test makes workspaces of its own.

func starterDesk(t *testing.T) work.Starter {
	t.Helper()
	ctx := context.Background()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed))
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	return work.Starter{
		Starts: postgres.NewStarterRepository(), Permits: authoriser, Authorizer: authoriser,
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
	}
}

// freshWorkspace is a new workspace with one person holding the role.
func freshWorkspace(ctx context.Context, t *testing.T, role string) (shared.ID, appshared.ActorContext) {
	t.Helper()
	tenant := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Fresh')`,
		tenant.String(), "fresh-"+tenant.String()[24:]); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	return tenant, personIn(ctx, t, tenant, role)
}

func personIn(ctx context.Context, t *testing.T, tenant shared.ID, role string) appshared.ActorContext {
	t.Helper()
	account := seedAccount(ctx, t, tenant)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', $4)`,
		freshID(t).String(), tenant.String(), account.String(), role); err != nil {
		t.Fatalf("granting %s: %v", role, err)
	}
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, AccountName: "Anna Beispiel",
		Scopes: []string{"containers:read", "containers:write", "workspace:manage"},
	}
}

func offered(ctx context.Context, t *testing.T, starter work.Starter, actor appshared.ActorContext) bool {
	t.Helper()
	answer, err := starter.StarterOffered(ctx, actor)
	if err != nil {
		t.Fatalf("asking whether the first start is offered: %v", err)
	}
	return answer
}

func TestTheFirstStartIsOfferedOnceToWhoeverMayShapeANewWorkspace(t *testing.T) {
	ctx := context.Background()
	starter := starterDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	member := personIn(ctx, t, tenant, "MEMBER")
	_, other := freshWorkspace(ctx, t, "OWNER")

	if !offered(ctx, t, starter, owner) {
		t.Fatalf("the owner of a new workspace is not asked")
	}
	if offered(ctx, t, starter, member) {
		t.Errorf("a member who may not shape the workspace is asked")
	}
	if err := (work.CloseStarter{Starter: starter}).Execute(ctx, member); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member closing it answered %v, want forbidden", err)
	}

	registry, err := usecase.NewRegistry(nil, work.CloseStarter{Starter: starter}.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	controller := rest.NewRestController()
	controller.UseCases = registry
	for range 2 { // closing again changes nothing
		request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, owner), http.MethodPost,
			rest.APIBasePath+"/tenant/starter:close", nil)
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("POST /tenant/starter:close answered %d %s", recorder.Code, recorder.Body.String())
		}
	}
	if offered(ctx, t, starter, owner) {
		t.Errorf("the first start is offered again after it was closed")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'workspace.starter_closed' AND outcome = 'SUCCESS'`,
		tenant.String()); rows != 1 {
		t.Errorf("%d audit entries for closing the first start, want one", rows)
	}
	// Another workspace is untouched by the close.
	if !offered(ctx, t, starter, other) {
		t.Errorf("closing one workspace's first start closed another's")
	}
}

func TestAWorkspaceHoldingAHubIsNotAskedItsFirstStart(t *testing.T) {
	ctx := context.Background()
	starter := starterDesk(t)
	tenant, owner := freshWorkspace(ctx, t, "OWNER")
	hub := containerIn(tenant, owner.AccountID, freshID(t), freshName(t), "a0")
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, hub)
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}
	if offered(ctx, t, starter, owner) {
		t.Errorf("a workspace holding a hub is asked its first start")
	}
}
