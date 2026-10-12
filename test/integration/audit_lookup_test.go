// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// Looking up what happened (UC-AUD-01) through the doors a reader actually uses, over a trail the
// real sink wrote. The filters themselves are asserted against the repository in
// audit_read_test.go; what is asked here is what the use case and the channels make of them.

func auditCatalogue(t *testing.T, list auditservice.ListAuditEntries) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, list.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func listerFor(ctx context.Context, t *testing.T, authorizer auditservice.Authorizer) auditservice.ListAuditEntries {
	t.Helper()
	return auditservice.ListAuditEntries{
		Trail: auditTrailRepo(), Authorizer: authorizer, Pseudonyms: privacyRepo(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
	}
}

// auditOverREST reads GET /audit and answers the entries' identifiers.
func auditOverREST(
	ctx context.Context, t *testing.T, registry *usecase.Registry,
	actor appshared.ActorContext, query url.Values,
) []string {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry

	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		http.MethodGet, rest.APIBasePath+"/audit?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /audit answered %d: %s", recorder.Code, recorder.Body)
	}

	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("GET /audit is not the contract's shape: %v", err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

// auditOverMCP calls the tool the catalogue derives for the read and answers the identifiers.
func auditOverMCP(
	ctx context.Context, t *testing.T, registry *usecase.Registry,
	actor appshared.ActorContext, arguments map[string]any,
) []string {
	t.Helper()
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}

	call, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name": auditservice.ListAuditEntries{}.Descriptor().MCPTool(), "arguments": arguments,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		http.MethodPost, mcp.Path, strings.NewReader(string(call)))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the MCP channel answered %d: %s", recorder.Code, recorder.Body)
	}

	var answer struct {
		Result struct {
			IsError    bool `json:"isError"`
			Structured struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the MCP answer is not JSON: %v", err)
	}
	if answer.Result.IsError {
		t.Fatalf("the tool call failed: %s", recorder.Body)
	}
	ids := make([]string, 0, len(answer.Result.Structured.Data))
	for _, entry := range answer.Result.Structured.Data {
		ids = append(ids, entry.ID)
	}
	return ids
}

// UC-AUD-01 check 1: the same filters give the same entries through every door - the use case,
// REST and MCP, all over one trail a real database holds.
func TestEveryDoorAnswersTheSameEntriesForTheSameFilter(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	entries := mixedEntries(t, tenant, 60)
	appendTo(ctx, t, tenant, entries)

	list := listerFor(ctx, t, permissive{})
	registry := auditCatalogue(t, list)
	actor := auditActor(tenant)

	from, to := created, created.Add(50*time.Second)
	page, err := list.Execute(ctx, actor, auditservice.EntryQuery{Filter: repository.Filter{
		From: from, To: to, ActionPrefix: "container.", ActorID: readerE,
		TargetType: "container", Outcome: port.OutcomeDenied, Page: repository.Page{Size: 500},
	}})
	if err != nil {
		t.Fatalf("reading through the use case: %v", err)
	}
	direct := make([]string, 0, len(page.Records))
	for _, record := range page.Records {
		direct = append(direct, record.ID.String())
	}
	if len(direct) == 0 {
		t.Fatal("the filter answered nothing, which proves nothing about parity")
	}

	overREST := auditOverREST(ctx, t, registry, actor, url.Values{
		"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)},
		"action": {"container."}, "actor_id": {readerE.String()}, "target_type": {"container"},
		"outcome": {string(port.OutcomeDenied)}, "limit": {"500"},
	})
	overMCP := auditOverMCP(ctx, t, registry, actor, map[string]any{
		"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339),
		"action": "container.", "actor_id": readerE.String(), "target_type": "container",
		"outcome": string(port.OutcomeDenied), "size": 500,
	})

	if !slices.Equal(overREST, direct) {
		t.Errorf("REST answered %v, the use case %v", overREST, direct)
	}
	if !slices.Equal(overMCP, direct) {
		t.Errorf("MCP answered %v, the use case %v", overMCP, direct)
	}
}

// UC-AUD-01 check 3, against the database after a real erasure: every entry of the erased actor
// names one and the same pseudonym - the one the erasure recorded - and none carries the former
// name or address, whichever of the two the entry was written with.
func TestAnErasedActorIsOneStablePseudonymOnEveryEntry(t *testing.T) {
	ctx := context.Background()
	email := "anna-" + freshID(t).String() + "@example.org"
	subject := seedSubject(ctx, t, email)

	bystander := freshID(t)
	var entries []port.Entry
	for i, label := range []string{"Anna Beispiel", "Anna Beispiel", email, "Anna Beispiel", email} {
		entries = append(entries, port.Entry{
			TenantID: subject.tenant, OccurredAt: created.Add(-time.Duration(i) * time.Minute),
			Action: "container.renamed", Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
			ActorKind: shared.ActorUser, ActorID: subject.account, ActorLabel: label,
			TargetType: "container", TargetID: subject.collection,
		})
	}
	entries = append(entries, port.Entry{
		TenantID: subject.tenant, OccurredAt: created.Add(-time.Hour),
		Action: "container.renamed", Outcome: port.OutcomeSuccess, Severity: port.SeverityInfo,
		ActorKind: shared.ActorUser, ActorID: bystander, ActorLabel: "Bert Beispiel",
		TargetType: "container", TargetID: subject.collection,
	})
	appendTo(ctx, t, subject.tenant, entries)

	if _, err := eraserFor(t).Erase(ctx, subjectActor(subject),
		erasureRequest(ctx, t, subject, domain.ModeFullDelete)); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	var recorded string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT pseudonym FROM audit_pseudonym WHERE tenant_id = $1 AND actor_id = $2`,
		subject.tenant.String(), subject.account.String()).Scan(&recorded); err != nil {
		t.Fatalf("reading the recorded pseudonym: %v", err)
	}
	if recorded == "" || recorded == "Anna Beispiel" || recorded == email {
		t.Fatalf("the erasure recorded %q as the pseudonym", recorded)
	}

	// Through the handler every channel calls, so that what is asserted is the projection a reader
	// receives rather than a record on its way to it.
	list := listerFor(ctx, t, permissive{})
	out, err := auditCatalogue(t, list).Invoke(ctx, auditservice.ListAuditEntriesName, auditActor(subject.tenant),
		usecase.Input{"target_id": subject.collection.String(), "size": 500})
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)

	erased, bystanderLabel := 0, ""
	for _, row := range rows {
		actor, _ := row["actor"].(map[string]any)
		label, _ := actor["label"].(string)
		if actor["id"] != subject.account.String() {
			bystanderLabel = label
			continue
		}
		erased++
		if label != recorded {
			t.Errorf("an entry of the erased actor names %q, want the recorded pseudonym %q", label, recorded)
		}
		serialised, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		for _, former := range []string{"Anna Beispiel", email} {
			if strings.Contains(string(serialised), former) {
				t.Errorf("an entry of the erased actor still carries %q: %s", former, serialised)
			}
		}
	}
	if erased != 5 {
		t.Errorf("%d entries of the erased actor came back, want the 5 written", erased)
	}
	if bystanderLabel != "Bert Beispiel" {
		t.Errorf("an actor nobody erased is answered as %q", bystanderLabel)
	}
}

// UC-AUD-01 check 6: reading the trail adds no entry. Asked of the database, through REST, with
// the real authorisation service and an owner's real membership - the one path a read could leave
// a row behind on without the use case writing one itself.
func TestReadingTheTrailAddsNoEntry(t *testing.T) {
	ctx := context.Background()
	tenant := auditTenant(ctx, t)
	owner := freshID(t)

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Olga Owner')`,
		owner.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the owner's account: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
		freshID(t).String(), tenant.String(), owner.String()); err != nil {
		t.Fatalf("granting the OWNER role: %v", err)
	}
	appendTo(ctx, t, tenant, mixedEntries(t, tenant, 12))

	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)),
		Audit:       postgres.NewAuditSink(generator{t}),
		Clock:       portclock.Fixed(created),
	}
	registry := auditCatalogue(t, listerFor(ctx, t, authorizer))
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: owner,
		AccountName: "Olga Owner", Scopes: []string{"audit:read"},
	}

	count := `SELECT count(*) FROM audit_log WHERE tenant_id = $1`
	before := countIn(ctx, t, count, tenant.String())

	first := auditOverREST(ctx, t, registry, actor, url.Values{"limit": {"500"}})
	second := auditOverREST(ctx, t, registry, actor, url.Values{"limit": {"500"}})

	if after := countIn(ctx, t, count, tenant.String()); after != before {
		t.Errorf("reading the trail twice left %d entries behind", after-before)
	}
	if len(first) != before || !slices.Equal(first, second) {
		t.Errorf("the second read saw %d entries, the first %d, the trail holds %d",
			len(second), len(first), before)
	}
}
