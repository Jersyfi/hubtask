// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// readsHubThroughREST asks GET /containers/{id} as the account and answers the status.
func readsHubThroughREST(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext, id shared.ID) int {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodGet,
		rest.APIBasePath+"/containers/"+id.String(), nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder.Code
}

// readsHubThroughMCP calls the get_container tool as the account and answers whether it failed.
func readsHubThroughMCP(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext, id shared.ID) bool {
	t.Helper()
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_container",` +
		`"arguments":{"container_id":"` + id.String() + `"}}}`
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPost, mcp.Path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var answer struct {
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the MCP answer is not JSON (%d): %s", recorder.Code, recorder.Body)
	}
	return answer.Error != nil || answer.Result == nil || answer.Result.IsError
}

// UC-SVC-05 check 11 (E-13): an account holding REQUEST at a desk and READ nowhere gets the
// portal's frame in the web app, and its REST and MCP requests for the hub around the desk are
// refused as before, while a reader of the same hub reads it through both doors.
func TestARequesterOnlyAccountIsRefusedTheHubAtEachDoor(t *testing.T) {
	ctx := context.Background()
	workspace, author := seedOwnTenant(ctx, t)
	ids := make([]shared.ID, 9)
	for i := range ids {
		ids[i] = freshID(t)
	}
	reader, requester, readRole, requestRole, hub, desk, readGrant, requestGrant := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6], ids[7]
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Rita')`, []any{reader.String(), workspace.String()}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Hannah')`, []any{requester.String(), workspace.String()}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Reader', ARRAY['READ'], now(), now())`,
			[]any{readRole.String(), workspace.String(), "reader_" + readRole.String()[30:]}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Customer', ARRAY['REQUEST'], now(), now())`,
			[]any{requestRole.String(), workspace.String(), "customer_" + requestRole.String()[30:]}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Service', 'a0', $3)`, []any{hub.String(), workspace.String(), author.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Help desk', 'a0', $4)`,
			[]any{desk.String(), workspace.String(), hub.String(), author.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'HUB', $4, $5, now())`,
			[]any{readGrant.String(), workspace.String(), reader.String(), hub.String(), readRole.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, $5, now())`,
			[]any{requestGrant.String(), workspace.String(), requester.String(), desk.String(), requestRole.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	if holdsAny(ctx, t, workspace, requester) {
		t.Fatal("the requester holds READ somewhere; the test would not ask what it means to")
	}

	registry := readerRegistry(ctx, t)
	as := func(account shared.ID) appshared.ActorContext {
		return appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: workspace, AccountID: account,
			AccountName: "Someone", Scopes: []string{"containers:read"},
		}
	}
	if code := readsHubThroughREST(ctx, t, registry, as(reader), hub); code != http.StatusOK {
		t.Fatalf("the hub's reader was answered %d through REST", code)
	}
	if failed := readsHubThroughMCP(ctx, t, registry, as(reader), hub); failed {
		t.Fatal("the hub's reader was refused through MCP")
	}
	for _, id := range []shared.ID{hub, desk} {
		if code := readsHubThroughREST(ctx, t, registry, as(requester), id); code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("the requester read %s through REST: %d", id, code)
		}
		if failed := readsHubThroughMCP(ctx, t, registry, as(requester), id); !failed {
			t.Errorf("the requester read %s through MCP", id)
		}
	}
}
