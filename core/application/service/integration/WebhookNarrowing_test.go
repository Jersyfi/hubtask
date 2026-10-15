// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

var (
	scopeHub        = shared.ID("01936f2a-7c1e-7000-8000-000000000f21")
	scopeCollection = shared.ID("01936f2a-7c1e-7000-8000-000000000f22")
	foreignHub      = shared.ID("01936f2a-7c1e-7000-8000-000000000f23")
	secondHook      = shared.ID("01936f2a-7c1e-7000-8000-000000000f24")
)

// compiler refuses a condition containing "!!" with the code an engine would name, and records
// the environment it was asked to compile against.
type compiler struct{ envs []expression.Environment }

func (c *compiler) Compile(text string, env expression.Environment, _ expression.Result) (expression.Program, error) {
	c.envs = append(c.envs, env)
	if strings.Contains(text, "!!") {
		return nil, shared.ErrValidation.WithDetail("expression.unknown_variable").
			WithParams(map[string]string{"name": "item"})
	}
	return nil, nil
}

// containers is this workspace's hub and collection; anything else is another workspace's.
type containers map[shared.ID]work.ContainerType

func (c containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	kind, found := c[id]
	if !found {
		return work.Container{}, shared.ErrNotFound
	}
	return work.Container{ID: id, TenantID: tenant, Type: kind}, nil
}

var _ condition.Containers = containers{}

func narrowingHarness() (*harness, *compiler) {
	h := newHarness()
	c := &compiler{}
	h.writer.Compiler = c
	h.writer.Containers = containers{scopeHub: work.ContainerHub, scopeCollection: work.ContainerCollection}
	return h, c
}

func lastPath(h *harness) []identity.Scope {
	return h.auth.requests[len(h.auth.requests)-1].Path
}

// UC-INT-01 check 9: a condition is compiled against the webhook's names when it is saved, and one
// that cannot be compiled is refused naming where and why - never stored to be ignored.
func TestAConditionIsCompiledWhenItIsSaved(t *testing.T) {
	h, c := narrowingHarness()
	create := validCreate()
	create.Filter = `data.title != "" !!`

	_, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), create)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "webhooks.filter_invalid" || !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want webhooks.filter_invalid", err)
	}
	if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/filter" ||
		refusal.Fields[0].Code != "expression.unknown_variable" {
		t.Errorf("fields = %+v, want /filter with the engine's code", refusal.Fields)
	}
	if len(h.store.rows) != 0 {
		t.Error("a refused condition was stored")
	}

	create.Filter = `data.title != ""`
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), create); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if got := h.store.rows[hookID].Subscription.Filter; got != create.Filter {
		t.Errorf("stored %q", got)
	}
	names := map[string]bool{}
	for _, variable := range c.envs[len(c.envs)-1].Variables {
		names[variable.Name] = true
	}
	if !names[condition.VarData] || !names[condition.VarEvent] || names[condition.VarItem] {
		t.Errorf("compiled against %v, want the webhook's names", names)
	}

	// Without an evaluator a condition is refused rather than stored on a promise nothing checks.
	bare := newHarness()
	if _, err := (CreateWebhookSubscription{Writer: bare.writer}).Execute(t.Context(), actor(), create); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v, want unavailable", err)
	}
}

// A scope is asked about where it listens, and its container must be this workspace's and of the
// level it names.
func TestAScopeIsAuthorisedAndCheckedWhereItListens(t *testing.T) {
	h, _ := narrowingHarness()
	create := validCreate()
	create.Scope = &domain.WebhookScope{Level: domain.ScopeHub, ID: scopeHub}

	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), create); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if path := lastPath(h); len(path) != 2 || path[1] != identity.HubScope(scopeHub) {
		t.Errorf("asked at %v, want the hub", path)
	}
	if got := h.store.rows[hookID].Subscription.Scope; got != *create.Scope {
		t.Errorf("stored scope %+v", got)
	}

	refused := map[string]struct {
		scope domain.WebhookScope
		code  string
	}{
		"another workspace's hub":   {domain.WebhookScope{Level: domain.ScopeHub, ID: foreignHub}, "containers.not_found"},
		"a hub named as collection": {domain.WebhookScope{Level: domain.ScopeCollection, ID: scopeHub}, "webhooks.scope_type_mismatch"},
	}
	for name, c := range refused {
		fresh, _ := narrowingHarness()
		create.Scope = &c.scope
		_, err := CreateWebhookSubscription{Writer: fresh.writer}.Execute(t.Context(), actor(), create)
		if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != c.code {
			t.Errorf("%s: error = %v, want %s", name, err, c.code)
		}
		if len(fresh.store.rows) != 0 {
			t.Errorf("%s: stored", name)
		}
	}
}

// The listing answers what the caller may read scope by scope: somebody without the permission on
// one hub does not see that hub's subscription.
func TestTheListingShowsWhatTheCallerMayReadThere(t *testing.T) {
	h, _ := narrowingHarness()
	create := validCreate()
	create.Scope = &domain.WebhookScope{Level: domain.ScopeHub, ID: scopeHub}
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), create); err != nil {
		t.Fatalf("creating: %v", err)
	}
	h.writer.IDs = ids{next: secondHook}
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), validCreate()); err != nil {
		t.Fatalf("creating the second: %v", err)
	}
	h.auth.deny = []shared.ID{scopeHub}

	listed, err := ListWebhookSubscriptions{Writer: h.writer}.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != secondHook {
		t.Errorf("listed %+v, want the workspace's subscription only", listed)
	}
}

// Moving a subscription asks both where it was and where it lands; TENANT widens it again, and an
// empty condition removes it. The trail says that a condition is set, never what it says.
func TestAnUpdateMovesTheScopeAndClearsTheCondition(t *testing.T) {
	h, _ := narrowingHarness()
	create := validCreate()
	create.Scope = &domain.WebhookScope{Level: domain.ScopeHub, ID: scopeHub}
	create.Filter = `data.secret_word == "plover"`
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), create); err != nil {
		t.Fatalf("creating: %v", err)
	}
	for _, entry := range h.sink.entries {
		if rendered := fmt.Sprintf("%v", entry.Changes); strings.Contains(rendered, "plover") {
			t.Fatalf("the condition's text reached the trail: %s", rendered)
		}
	}
	h.auth.requests = nil

	widened := domain.WebhookScope{Level: domain.ScopeTenant}
	none := ""
	updated, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, Scope: &widened, Filter: &none})
	if err != nil {
		t.Fatalf("updating: %v", err)
	}
	if !updated.Scope.IsWorkspace() || updated.Filter != "" {
		t.Errorf("updated to scope %+v, filter %q", updated.Scope, updated.Filter)
	}
	if len(h.auth.requests) != 2 || len(h.auth.requests[0].Path) != 2 || len(h.auth.requests[1].Path) != 1 {
		t.Errorf("asked at %+v, want the hub and then the workspace", h.auth.requests)
	}
}
