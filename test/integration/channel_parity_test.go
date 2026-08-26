// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// Test AT-6, in full: the same use case reached through REST, through MCP and as an automation
// action produces the same audit entry (audit.md §8).
//
// It is asked of the database rather than of the code, because that is where the claim lives. Forty
// source comments say "the entry happens in the application layer, once, whichever channel the call
// arrived through"; what makes that true is that all three doors open onto the same catalogue, and
// what would break it is a controller that wrote an entry of its own or skipped one.

// entryFor reads the one audit entry a container's creation left, in the fields that have to be
// the same whichever channel made it.
type auditFacts struct {
	action     string
	outcome    string
	severity   string
	targetType string
	actorType  string
	changes    string
}

func entryFor(ctx context.Context, t *testing.T, containerID string) auditFacts {
	t.Helper()

	var facts auditFacts
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT action, outcome, severity, target_type, actor_type, changes::text
		FROM audit_log WHERE target_id = $1`, containerID).Scan(
		&facts.action, &facts.outcome, &facts.severity,
		&facts.targetType, &facts.actorType, &facts.changes); err != nil {
		t.Fatalf("reading the audit entry of %s: %v", containerID, err)
	}
	return facts
}

// throughREST creates a hub the way a request does, and answers the container's identifier.
func throughREST(ctx context.Context, t *testing.T, registry *usecase.Registry, name string) string {
	t.Helper()

	controller := rest.NewRestController()
	controller.UseCases = registry

	actorCtx := appshared.ContextWithActor(ctx, administrator(tenantA, authorA))
	body := `{"type":"HUB","name":"` + name + `"}`
	request := httptest.NewRequestWithContext(actorCtx, http.MethodPost,
		rest.APIBasePath+"/containers", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("the REST channel answered %d: %s", recorder.Code, recorder.Body)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("the REST answer is not the contract's shape: %v", err)
	}
	return created.ID
}

// throughMCP creates one the way an agent does, over JSON-RPC.
func throughMCP(ctx context.Context, t *testing.T, registry *usecase.Registry, name string) string {
	t.Helper()

	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	agent := administrator(tenantA, authorA)
	agent.Kind = appshared.ActorAIAgent

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_container",` +
		`"arguments":{"type":"HUB","name":"` + name + `"}}}`
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, agent),
		http.MethodPost, mcp.Path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the MCP channel answered %d: %s", recorder.Code, recorder.Body)
	}

	var answer struct {
		Result struct {
			IsError    bool           `json:"isError"`
			Structured map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the MCP answer is not JSON: %v", err)
	}
	if answer.Result.IsError {
		t.Fatalf("the tool call failed: %s", recorder.Body)
	}
	id, _ := answer.Result.Structured["id"].(string)
	return id
}

// throughAutomation creates one the way a rule does.
func throughAutomation(ctx context.Context, t *testing.T, registry *usecase.Registry, name string) string {
	t.Helper()

	rule := administrator(tenantA, authorA)
	rule.Kind = appshared.ActorAutomation

	out, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind:   "CREATE_CONTAINER",
		Params: map[string]any{"type": "HUB", "name": name},
	})
	if err != nil {
		t.Fatalf("the automation channel failed: %v", err)
	}
	return out.String("id")
}

// shapeOf reduces the recorded changes to what has to be identical across the channels: which
// fields are there and how each was masked. The values cannot be compared - each channel creates a
// hub of its own, so the fingerprint of a `SENSITIVE` field differs by construction, and that
// difference is the masking working rather than a divergence.
func shapeOf(t *testing.T, changes string) string {
	t.Helper()

	var decoded map[string]map[string]any
	if err := json.Unmarshal([]byte(changes), &decoded); err != nil {
		t.Fatalf("the recorded changes are not an object: %v", err)
	}

	shape := make([]string, 0, len(decoded))
	for field, masked := range decoded {
		keys := make([]string, 0, len(masked))
		for key := range masked {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		shape = append(shape, field+":"+strings.Join(keys, "+"))
	}
	sort.Strings(shape)
	return strings.Join(shape, ",")
}

func TestTheThreeChannelsWriteTheSameAuditEntry(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)

	viaREST := entryFor(ctx, t, throughREST(ctx, t, registry, freshName(t)))
	viaMCP := entryFor(ctx, t, throughMCP(ctx, t, registry, freshName(t)))
	viaAutomation := entryFor(ctx, t, throughAutomation(ctx, t, registry, freshName(t)))

	// Everything but the actor type: an entry has to say which channel it came through, and
	// nothing else about the entry may depend on that.
	for _, other := range []auditFacts{viaMCP, viaAutomation} {
		if other.action != viaREST.action || other.outcome != viaREST.outcome {
			t.Errorf("the channels recorded %s/%s and %s/%s",
				viaREST.action, viaREST.outcome, other.action, other.outcome)
		}
		if other.severity != viaREST.severity || other.targetType != viaREST.targetType {
			t.Errorf("the channels recorded %s on %s and %s on %s",
				viaREST.severity, viaREST.targetType, other.severity, other.targetType)
		}
		if shapeOf(t, other.changes) != shapeOf(t, viaREST.changes) {
			t.Errorf("the channels recorded different changes:\n%s\n%s", viaREST.changes, other.changes)
		}
	}

	// And the one field that does differ, because an auditor has to be able to tell a person from
	// a rule from an agent (audit.md §2).
	if viaREST.actorType != string(appshared.ActorUser) {
		t.Errorf("a request was recorded as %s", viaREST.actorType)
	}
	if viaMCP.actorType != string(appshared.ActorAIAgent) {
		t.Errorf("an agent was recorded as %s", viaMCP.actorType)
	}
	if viaAutomation.actorType != string(appshared.ActorAutomation) {
		t.Errorf("a rule was recorded as %s", viaAutomation.actorType)
	}
}

// One entry per act, whichever channel it arrived through. Two would be a controller that recorded
// something of its own, and none would be a channel that skipped the application layer.
func TestEachChannelWritesExactlyOneEntry(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)

	for channel, create := range map[string]func(context.Context, *testing.T, *usecase.Registry, string) string{
		"REST":       throughREST,
		"MCP":        throughMCP,
		"automation": throughAutomation,
	} {
		id := create(ctx, t, registry, freshName(t))
		if id == "" {
			t.Fatalf("the %s channel created nothing", channel)
		}
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM audit_log WHERE target_id = $1`, id); rows != 1 {
			t.Errorf("%d audit entries for a container created through %s", rows, channel)
		}
	}
}

// Test AT-7: an entry stays readable after the account that made it is gone.
//
// The trail denormalises the actor's label for exactly this, and the table has no foreign key to
// `account` for the same reason. What would break it is somebody "tidying up" either.
func TestAnEntryOutlivesTheAccountThatMadeIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	leaver := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Bea Leaver')`,
		leaver.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'ADMIN')`,
		freshID(t).String(), tenantA.String(), leaver.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}

	actor := administrator(tenantA, leaver)
	actor.AccountName = "Bea Leaver"
	out, err := catalogueFor(t).Invoke(ctx, "CreateContainer", actor,
		usecase.Input{"type": "HUB", "name": freshName(t)})
	if err != nil {
		t.Fatalf("creating the hub: %v", err)
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM account WHERE id = $1`, leaver.String()); err != nil {
		t.Fatalf("deleting the account: %v", err)
	}

	var label string
	var actorID *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT actor_label, actor_id::text FROM audit_log WHERE target_id = $1`,
		out.String("id")).Scan(&label, &actorID); err != nil {
		t.Fatalf("the entry did not survive the deletion: %v", err)
	}
	if label != "Bea Leaver" {
		t.Errorf("the entry reads %q after the account was deleted", label)
	}
	// The identifier stays too, and points at nothing. That is the point of there being no
	// foreign key: an auditor correlating two entries by actor can still do so.
	if actorID == nil || *actorID != leaver.String() {
		t.Errorf("the entry names the actor %v", actorID)
	}
}
