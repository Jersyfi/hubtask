// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func askFixture(provider *translator) (AskQuery, *sink) {
	recorded := &sink{}
	return AskQuery{
		Providers: translatorResolver{provider: provider}, Prompts: translatePrompts{},
		Audit: transactionalSink{sink: recorded}, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}, recorded
}

// Q-6: a question becomes a query the person sees - read by the parser, answered canonical, run by
// nobody - and the provider is told the keys this installation answers.
func TestAQuestionBecomesAVisibleQuery(t *testing.T) {
	provider := &translator{completion: true, answer: "Sure: {\"query\": \"who:me  is:open due: <today\"}"}
	ask, recorded := askFixture(provider)

	out, err := ask.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"question": "what of mine is overdue?",
	})
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if out["query"] != "who: me is: open due: <today" || out["source"] != "AI" || out["model"] != "gpt-x" {
		t.Errorf("answered %v", out)
	}
	if len(provider.asked) != 1 {
		t.Fatalf("the provider was asked %d times", len(provider.asked))
	}
	var material strings.Builder
	for _, message := range provider.asked[0].Messages {
		material.WriteString(message.Content)
	}
	for _, wanted := range []string{"what of mine is overdue?", "- who (person)", "- plan.slip (number)"} {
		if !strings.Contains(material.String(), wanted) {
			t.Errorf("the material lacks %q", wanted)
		}
	}
	// Rule 10: the audit says a query was asked for, never the question.
	if len(recorded.entries) != 1 || recorded.entries[0].Action != QueryAskedAction {
		t.Fatalf("audited %+v", recorded.entries)
	}
	if strings.Contains(fmt.Sprint(recorded.entries[0]), "overdue") {
		t.Error("the question reached the audit")
	}
}

// Without AI, a slow or failing provider, or an answer that is no query: ai.unavailable.
func TestAQuestionWithoutAnAnswerIsUnavailable(t *testing.T) {
	cases := map[string]*translator{
		"no completion":   {completion: false},
		"a failing model": {completion: true, err: errors.New("boom")},
		"not json":        {completion: true, answer: "who: me"},
		"not a query":     {completion: true, answer: `{"query": "nonsense: (("}`},
	}
	for name, provider := range cases {
		t.Run(name, func(t *testing.T) {
			ask, _ := askFixture(provider)
			_, err := ask.Execute(t.Context(), actorFixture(), "anything open?")
			if !errors.Is(err, aiprovider.ErrUnavailable) {
				t.Errorf("answered %v, want ai.unavailable", err)
			}
		})
	}
	unwired := AskQuery{}
	if _, err := unwired.Execute(t.Context(), actorFixture(), "anything?"); !errors.Is(err, aiprovider.ErrUnavailable) {
		t.Errorf("a build without AI answered %v", err)
	}
}

func TestAQuestionIsBounded(t *testing.T) {
	ask, _ := askFixture(&translator{completion: true})
	for question, code := range map[string]string{
		"   ":                    "query.question_required",
		strings.Repeat("x", 501): "query.question_too_long",
	} {
		_, err := ask.Execute(t.Context(), actorFixture(), question)
		if err == nil || !strings.Contains(err.Error(), code) {
			t.Errorf("%q answered %v, want %s", question[:3], err, code)
		}
	}
}
