// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// refusesEverywhere refuses every permission: a person whose one role is on a hub, asked at the
// workspace.
type refusesEverywhere struct{}

func (refusesEverywhere) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

// countedIDs answers a new identifier each time.
type countedIDs struct{ next *int }

func (c countedIDs) NewID() shared.ID {
	*c.next++
	return shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-%012x", 0xe000+*c.next))
}

func askingTheWorkspace(available bool) (*usecase.Registry, asker, *requestStore) {
	a := newAsker(available)
	requests := newRequestStore()
	a.ask.Cases.Requests, a.ask.Cases.IDs = requests, countedIDs{next: new(int)}
	a.ask.Cases.Authorizer = refusesEverywhere{}
	registry, err := usecase.NewRegistry(nil, AskWorkspace(a.ask).Descriptor())
	if err != nil {
		panic(err)
	}
	return registry, a, requests
}

func questioner() appshared.ActorContext {
	actor := person()
	actor.Scopes = []string{suggestionsRead, suggestionsWrite}
	return actor
}

// A question is held for its job, queued under its own kind about the asker's account, and recorded
// without its words - by somebody the workspace refuses every permission, because a role on one hub
// is enough to ask and what they may read is decided entry by entry (UC-AI-08 check 3).
func TestAskingTheWorkspaceQueuesItsOwnJobAboutTheAsker(t *testing.T) {
	registry, a, requests := askingTheWorkspace(true)

	out, err := registry.Invoke(context.Background(), AskWorkspaceName, questioner(), usecase.Input{
		"question": "  What is open for Müller GmbH this week?  ",
	})
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if out.String("job_id") == "" || len(a.jobs.queued) != 1 {
		t.Fatalf("no job to follow: %v, %d", out, len(a.jobs.queued))
	}
	job := a.jobs.queued[0]
	if job.Kind != queue.KindAiAnswer || job.MaxAttempts != queue.AiDraftRuleAttempts {
		t.Errorf("job %s with %d attempts", job.Kind, job.MaxAttempts)
	}
	for key, want := range map[string]any{
		"target_type": "ACCOUNT", "target_id": accountID.String(), "kind": "ANSWER",
		"prompt": answerPrompt, "asked_by": accountID.String(),
	} {
		if job.Payload[key] != want {
			t.Errorf("payload %s = %v, want %v", key, job.Payload[key], want)
		}
	}
	held, found := requests.held[shared.ID(fmt.Sprint(job.Payload["request_id"]))]
	if !found || held.Text != "What is open for Müller GmbH this week?" {
		t.Errorf("held %+v", held)
	}
	for key, value := range job.Payload {
		if text, isText := value.(string); isText && strings.Contains(text, "Müller") {
			t.Errorf("the payload carries words under %s", key)
		}
	}
	if len(a.world.entries) != 1 || a.world.entries[0].Action != QuestionAskedAction ||
		a.world.entries[0].TargetType != accountTarget || a.world.entries[0].TargetID != accountID {
		t.Errorf("trail %+v", a.world.entries)
	}
}

// Each question stands alone (UC-AI-08 check 5): asking again holds the new words under a new
// request, and nothing of the first travels with the second.
func TestEachQuestionStandsAlone(t *testing.T) {
	registry, a, requests := askingTheWorkspace(true)
	for _, words := range []string{"What is open for Müller?", "And for Schmidt?"} {
		if _, err := registry.Invoke(context.Background(), AskWorkspaceName, questioner(),
			usecase.Input{"question": words}); err != nil {
			t.Fatalf("asking %q: %v", words, err)
		}
	}
	if len(a.jobs.queued) != 2 {
		t.Fatalf("%d jobs", len(a.jobs.queued))
	}
	first, second := a.jobs.queued[0].Payload, a.jobs.queued[1].Payload
	if first["request_id"] == second["request_id"] {
		t.Error("the second question was held under the first one's request")
	}
	if held := requests.held[shared.ID(fmt.Sprint(second["request_id"]))]; held.Text != "And for Schmidt?" {
		t.Errorf("the second question holds %q", held.Text)
	}
	for key := range second {
		if _, ordinary := first[key]; !ordinary {
			t.Errorf("the second question carries %s, which the first did not", key)
		}
	}
}

func TestAskingTheWorkspaceIsRefusedWithoutItsConditions(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		available bool
		actor     func() appshared.ActorContext
		question  string
		detail    string
		category  error
	}{
		{"no provider or consent", false, questioner, "What is open?", "ai.unavailable", shared.ErrUnavailable},
		{"a token without items:write", true, func() appshared.ActorContext {
			actor := questioner()
			actor.Scopes = []string{suggestionsRead, automationScope}
			return actor
		}, "What is open?", "access.insufficient_scope", shared.ErrForbidden},
		{"no words", true, questioner, "   ", "usecase.input_invalid", shared.ErrValidation},
		{"more words than a question holds", true, questioner, strings.Repeat("a", MaxTemplateDescription+1),
			"suggestions.question_invalid", shared.ErrValidation},
	} {
		registry, a, _ := askingTheWorkspace(testCase.available)
		_, err := registry.Invoke(context.Background(), AskWorkspaceName, testCase.actor(),
			usecase.Input{"question": testCase.question})
		if detail := shared.AsError(err); detail == nil || detail.DetailCode != testCase.detail ||
			!errors.Is(err, testCase.category) {
			t.Errorf("%s: %v, want %s", testCase.name, err, testCase.detail)
		}
		if len(a.jobs.queued) != 0 {
			t.Errorf("%s: a job was queued", testCase.name)
		}
	}
}

// A question that finds nothing the asker may read is refused at once, not queued for a provider
// to answer from its words alone.
func TestAQuestionWithNothingToAnswerFromIsNotQueued(t *testing.T) {
	a := newAsker(true)
	a.ask.Cases.Requests, a.ask.Cases.IDs = newRequestStore(), countedIDs{next: new(int)}
	a.ask.Cases.Material = nothingFound{}
	_, err := AskWorkspace(a.ask).Execute(context.Background(), questioner(), "What about zzz?")
	if detail := shared.AsError(err); detail == nil || detail.DetailCode != "ai.nothing_to_work_with" {
		t.Errorf("answered %v", err)
	}
	if len(a.jobs.queued) != 0 {
		t.Error("a job was queued")
	}
}

// nothingFound is a workspace where a question names no place and its words find no entry.
type nothingFound struct{}

func (nothingFound) Material(context.Context, appshared.ActorContext, Question) (Material, error) {
	return Material{}, nil
}
