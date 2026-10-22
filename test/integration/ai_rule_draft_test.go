// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	suggestionrepo "github.com/Jersyfi/hubtask/core/application/repository/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A rule drafted from a sentence (UC-AUT-10) against PostgreSQL: the row the schema now admits, read
// only by the person who asked - under row level security and the draft's own check - and accepted
// into a rule that is stored switched off.
func TestARuleDraftIsItsAskersAloneAndIsSavedSwitchedOff(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)

	rules := &creatingRules{t: t}
	cases := suggestionCases(ctx, t)
	cases.Catalogue = rules

	draft, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: freshID(t), TenantID: tenantA,
		TargetType: suggestiondomain.TargetAccount, TargetID: authorA, Kind: suggestiondomain.KindRule,
		Payload: map[string]any{
			"scope":       map[string]any{"type": "TENANT"},
			"trigger":     map[string]any{"kind": "EVENT", "event_type": string(event.ItemCreated)},
			"actions":     []any{map[string]any{"kind": "COMPLETE_WORK_ITEM"}},
			"not_drafted": []any{map[string]any{"part": "set it due in two days"}},
		},
		Provenance: suggestiondomain.Provenance{
			Model: "a-model", PromptID: "draft-rule", PromptVersion: "v1", ProducedAt: created,
		},
		InputDigest: suggestiondomain.Digest(authorA.String(), ""),
		Now:         created,
	})
	if err != nil {
		t.Fatalf("building the draft: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return cases.Suggestions.Record(ctx, draft)
	}); err != nil {
		t.Fatalf("the schema refused a rule draft about its asker: %v", err)
	}

	asker := drafting(tenantA, authorA)
	page, err := (suggestion.ListSuggestions{Cases: cases}).Execute(ctx, asker, suggestion.ListQuery{
		TargetType: suggestiondomain.TargetAccount, TargetID: authorA,
	})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("the asker does not see their draft: %v, %d", err, len(page.Items))
	}

	// Another workspace's owner naming the draft: row level security answers nothing, so the
	// draft is unknown there - not found, as for an entry they cannot see.
	// A workspace and an owner of their own: the package's tests share one database, and a role
	// given in a shared fixture workspace is one the other tests count.
	other, owner := freshID(t), freshID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Elsewhere')`,
			[]any{other.String(), "drafts-" + other.String()[24:]}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Bo')`,
			[]any{owner.String(), other.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), other.String(), owner.String()}},
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the other workspace's owner: %v", err)
		}
	}
	stranger := drafting(other, owner)
	if _, err := (suggestion.AcceptSuggestion{Cases: cases}).Execute(ctx, stranger, draft.ID,
		map[string]any{"name": "x", "run_as": runAs.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another workspace's account was answered %v", err)
	}
	if _, err := (suggestion.ListSuggestions{Cases: cases}).Execute(ctx, stranger, suggestion.ListQuery{
		TargetType: suggestiondomain.TargetAccount, TargetID: authorA,
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another workspace's account listed the draft: %v", err)
	}

	accepted, createdID, err := (suggestion.AcceptSuggestion{Cases: cases}).Accept(ctx, asker, draft.ID,
		map[string]any{"name": "Close new tasks", "run_as": runAs.String()})
	if err != nil {
		t.Fatalf("the draft was not accepted: %v", err)
	}
	if accepted.Status != suggestiondomain.StatusAccepted || createdID.IsZero() {
		t.Fatalf("accepted %s, created %s", accepted.Status, createdID)
	}
	var stored domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = automationRules().Find(ctx, createdID)
		return findErr
	}); err != nil {
		t.Fatalf("the rule is not there: %v", err)
	}
	if stored.Enabled {
		t.Error("the rule a draft became was stored switched on")
	}
	if stored.Name != "Close new tasks" || stored.RunAs != runAs {
		t.Errorf("stored %+v", stored)
	}
}

func drafting(tenant, account shared.ID) appshared.ActorContext {
	actor := reader(tenant, account)
	actor.Scopes = append(actor.Scopes, "items:write", "automation:manage")
	return actor
}

// creatingRules is CreateRule's write with nothing in front of it: the rule built by the aggregate
// from the input the acceptance hands over, and inserted in the acceptance's own transaction. What
// CreateRule checks before it - the permission, the composition rule - is its own suite's.
type creatingRules struct{ t *testing.T }

func (c *creatingRules) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if name != "CreateRule" {
		return nil, shared.ErrNotFound
	}
	runAs, err := in.ID("run_as")
	if err != nil {
		return nil, err
	}
	trigger, _ := in["trigger"].(map[string]any)
	eventType, _ := trigger["event_type"].(string)
	var actions []domain.Action
	for _, step := range in["actions"].([]any) {
		action, _ := step.(map[string]any)
		kind, _ := action["kind"].(string)
		actions = append(actions, domain.Action{Kind: kind})
	}
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(c.t), TenantID: actor.TenantID, Name: in.String("name"),
		Scope: domain.Scope{Type: domain.ScopeTenant}, RunAs: runAs,
		Trigger: domain.Trigger{Kind: domain.TriggerEvent, EventType: event.Type(eventType)},
		Actions: actions, CreatedBy: actor.AccountID, Now: time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	if err := automationRules().Insert(ctx, rule); err != nil {
		return nil, err
	}
	return usecase.Output{"id": rule.ID.String(), "enabled": rule.Enabled}, nil
}

// An erasure takes what AI drafted for the person alone and the words a job still holds for them;
// another person's draft stays.
func TestAnErasureTakesThePersonsDraftsAndHeldWords(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subject := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Erin')`,
		subject.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}
	store := postgres.NewSuggestionRepository(pageCursors())
	draftFor := func(account shared.ID) suggestiondomain.Suggestion {
		draft, err := suggestiondomain.New(suggestiondomain.NewInput{
			ID: freshID(t), TenantID: tenantA,
			TargetType: suggestiondomain.TargetAccount, TargetID: account, Kind: suggestiondomain.KindRule,
			Payload: map[string]any{"scope": map[string]any{"type": "TENANT"}},
			Provenance: suggestiondomain.Provenance{
				Model: "a-model", PromptID: "draft-rule", PromptVersion: "v1", ProducedAt: created,
			},
			InputDigest: suggestiondomain.Digest(account.String(), ""), Now: created,
		})
		if err != nil {
			t.Fatalf("building a draft: %v", err)
		}
		return draft
	}
	theirs, another := draftFor(subject), draftFor(authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, draft := range []suggestiondomain.Suggestion{theirs, another} {
			if err := store.Record(ctx, draft); err != nil {
				return err
			}
		}
		return store.Put(ctx, suggestionrepo.Request{
			ID: freshID(t), AskedBy: subject, Text: "When a task arrives, give it to Lisa", CreatedAt: created,
		})
	}); err != nil {
		t.Fatalf("seeding the drafts: %v", err)
	}

	var removed int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		removed, err = postgres.NewPrivacyRepository(pageCursors()).DiscardAiDrafts(ctx, subject)
		return err
	}); err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if removed != 2 {
		t.Errorf("%d rows removed, want the draft and the held words", removed)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := store.Find(ctx, theirs.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the person's draft is still there: %v", err)
		}
		if _, err := store.Find(ctx, another.ID); err != nil {
			t.Errorf("somebody else's draft went too: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
