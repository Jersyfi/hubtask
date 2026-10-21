// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// automationOnHub permits automation on one hub and below it, and nowhere else.
type automationOnHub struct{ hub shared.ID }

func (a automationOnHub) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	if request.Permission != service.PermissionAutomation {
		return nil
	}
	for _, at := range request.Path {
		if at.Type == identity.ScopeHub && at.ID == a.hub {
			return nil
		}
	}
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

func describing(available bool) (*usecase.Registry, asker, *requestStore) {
	a := newAsker(available)
	requests := newRequestStore()
	a.ask.Cases.Requests, a.ask.Cases.IDs = requests, sequentialIDs{}
	a.ask.Cases.Authorizer = automationOnHub{hub: containerTargetID}
	registry, err := usecase.NewRegistry(nil, DescribeRule(a.ask).Descriptor())
	if err != nil {
		panic(err)
	}
	return registry, a, requests
}

// Asking for a draft holds the sentence, queues its own job kind with the scope as references, and
// records the asking with the scope - never the words.
func TestDescribingARuleQueuesItsOwnJobWithTheScope(t *testing.T) {
	registry, a, requests := describing(true)

	out, err := registry.Invoke(context.Background(), DescribeRuleName, drafter(), usecase.Input{
		"scope":       map[string]any{"type": "HUB", "id": containerTargetID.String()},
		"description": "  When a task is created, give it to Lisa.  ",
	})
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if out.String("job_id") == "" {
		t.Error("no job to follow")
	}
	if len(a.jobs.queued) != 1 {
		t.Fatalf("%d jobs", len(a.jobs.queued))
	}
	job := a.jobs.queued[0]
	if job.Kind != queue.KindAiDraftRule || job.MaxAttempts != queue.AiDraftRuleAttempts {
		t.Errorf("job %s with %d attempts", job.Kind, job.MaxAttempts)
	}
	for key, want := range map[string]any{
		"target_type": "ACCOUNT", "target_id": accountID.String(), "kind": "RULE",
		"prompt": draftRulePrompt, "scope_type": "HUB", "scope_id": containerTargetID.String(),
	} {
		if job.Payload[key] != want {
			t.Errorf("payload %s = %v, want %v", key, job.Payload[key], want)
		}
	}
	for _, held := range requests.held {
		if held.Text != "When a task is created, give it to Lisa." {
			t.Errorf("held %q", held.Text)
		}
	}
	for key, value := range job.Payload {
		if text, isText := value.(string); isText && len(text) > 40 {
			t.Errorf("the payload carries words under %s: %q", key, text)
		}
	}
	if len(a.world.entries) != 1 || a.world.entries[0].Action != RuleAskedAction ||
		a.world.entries[0].TargetType != ruleTarget || a.world.entries[0].TargetID != containerTargetID {
		t.Errorf("trail %+v", a.world.entries)
	}
}

func TestDescribingARuleIsRefusedWhereWritingOneWouldBe(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		available bool
		actor     func() appshared.ActorContext
		in        usecase.Input
		detail    string
		category  error
	}{
		{"no provider or consent", false, drafter,
			usecase.Input{"scope": map[string]any{"type": "HUB", "id": containerTargetID.String()}, "description": "x"},
			"ai.unavailable", shared.ErrUnavailable},
		{"the whole workspace, by somebody who automates one hub", true, drafter,
			usecase.Input{"scope": map[string]any{"type": "TENANT"}, "description": "x"},
			"access.not_permitted", shared.ErrForbidden},
		{"a token without automation:manage", true, func() appshared.ActorContext {
			actor := drafter()
			actor.Scopes = []string{suggestionsWrite}
			return actor
		}, usecase.Input{"scope": map[string]any{"type": "HUB", "id": containerTargetID.String()}, "description": "x"},
			"access.insufficient_scope", shared.ErrForbidden},
		{"no words", true, drafter,
			usecase.Input{"scope": map[string]any{"type": "HUB", "id": containerTargetID.String()}, "description": "   "},
			"usecase.input_invalid", shared.ErrValidation},
		{"more words than a question holds", true, drafter,
			usecase.Input{"scope": map[string]any{"type": "HUB", "id": containerTargetID.String()}, "description": strings.Repeat("a", MaxTemplateDescription+1)},
			"suggestions.description_invalid", shared.ErrValidation},
		{"a scope of no known kind", true, drafter,
			usecase.Input{"scope": map[string]any{"type": "PLANET"}, "description": "x"},
			"automation.scope_type_unknown", shared.ErrValidation},
	} {
		registry, a, _ := describing(testCase.available)
		_, err := registry.Invoke(context.Background(), DescribeRuleName, testCase.actor(), testCase.in)
		if detail := shared.AsError(err); detail == nil || detail.DetailCode != testCase.detail ||
			!errors.Is(err, testCase.category) {
			t.Errorf("%s: %v, want %s", testCase.name, err, testCase.detail)
		}
		if len(a.jobs.queued) != 0 {
			t.Errorf("%s: a job was queued", testCase.name)
		}
	}
}
