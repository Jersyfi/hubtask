// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"slices"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

const (
	ritaID     = "01a0e2e0-0000-7000-8000-000000000005"
	openOwnID  = "01a0e2e0-0000-7000-8000-0000000000e1"
	byKimID    = "01a0e2e0-0000-7000-8000-0000000000e2"
	namesKimID = "01a0e2e0-0000-7000-8000-0000000000e3"
	onRitaID   = "01a0e2e0-0000-7000-8000-0000000000e4"
)

// askWorkspace is ruleWorkspace with entries: what SearchItems answers the asker. An entry in a hub
// the asker holds nothing on is one the search does not answer them - that is the read's own
// narrowing, which the integration test proves against the database.
type askWorkspace struct {
	ruleWorkspace
	searched *[]usecase.Input
}

func (w askWorkspace) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if name != "SearchItems" {
		return w.ruleWorkspace.Invoke(ctx, name, actor, in)
	}
	if w.searched != nil {
		*w.searched = append(*w.searched, in)
	}
	if in.String("container_id") == websiteID {
		return page(
			usecase.Output{"id": openOwnID, "title": "Send the offer to Müller GmbH", "collection_id": websiteID,
				"created_by": askerID, "assignee_id": lisaID, "due_at": "2026-10-28T00:00:00Z",
				"notes": "Second draft.\nWith prices."},
			usecase.Output{"id": byKimID, "title": "Kim's private plan", "collection_id": websiteID,
				"created_by": kimID},
		), nil
	}
	if in.String("q") != "" {
		return page(
			// Found by the words as well as in the named collection: shown once.
			usecase.Output{"id": openOwnID, "title": "Send the offer to Müller GmbH", "collection_id": websiteID,
				"created_by": askerID},
			usecase.Output{"id": namesKimID, "title": "Ask Kim Lisa about Müller", "collection_id": gardenID,
				"created_by": lisaID},
			usecase.Output{"id": onRitaID, "title": "Müller invoice", "collection_id": gardenID,
				"created_by": lisaID, "assignee_id": ritaID},
		), nil
	}
	return page(), nil
}

// keepsKimRestrictsRita keeps Kim's content out by objection, and restricts Rita's processing.
type keepsKimRestrictsRita struct{}

func (keepsKimRestrictsRita) Read(context.Context, shared.ID) (integration.KeptOut, error) {
	return integration.NewKeptOut([]integration.KeptPerson{
		{AccountID: shared.ID(kimID), Reason: integration.KeptOutObjected, Names: []string{"Kim Lisa"}},
		{AccountID: shared.ID(ritaID), Reason: integration.KeptOutRestricted, Names: []string{"Rita Stein"}},
	}), nil
}

func askSources(searched *[]usecase.Input) (CatalogueSources, appshared.ActorContext) {
	sources, actor := ruleSources()
	sources.Catalogue = askWorkspace{searched: searched}
	sources.Kept = keepsKimRestrictsRita{}
	return sources, actor
}

func askQuestion(actor appshared.ActorContext, words string) Question {
	return Question{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID, PromptID: answerPrompt, Words: words,
	}
}

// The material is what the question names and what its words find, read as the asker; a kept-out
// person's entries and a restricted person's are not in it, and their names are scrubbed from
// what is (UC-AI-08 check 3).
func TestAQuestionIsAnsweredFromReadableEntriesWithoutKeptOutContent(t *testing.T) {
	var searched []usecase.Input
	sources, actor := askSources(&searched)
	material, err := sources.Material(context.Background(), actor,
		askQuestion(actor, "What is open in Website relaunch for Müller this week, Lisa?"))
	if err != nil {
		t.Fatalf("the material could not be read: %v", err)
	}
	sets := material.Answer
	if sets == nil {
		t.Fatal("an answer's material carries no sets")
	}
	if !slices.Equal(sets.Entries, []string{openOwnID, namesKimID}) {
		t.Errorf("entries %v: the open one in the named collection and the one the words find, once each; "+
			"not Kim's, who keeps their content out, nor the one assigned to Rita, whose processing is restricted", sets.Entries)
	}
	if len(sets.Places) != 1 || sets.Places[0].ID != websiteID || sets.Places[0].Type != "COLLECTION" {
		t.Errorf("places %v", sets.Places)
	}
	if got := optionIDs(sets.People); !slices.Equal(got, []string{askerID, lisaID}) {
		t.Errorf("people %v: the asker and Lisa, whom the question names", got)
	}
	for _, absent := range []string{"Kim", "Rita", "private plan", "invoice", byKimID, onRitaID} {
		if strings.Contains(material.Content, absent) {
			t.Errorf("the material shows %q", absent)
		}
	}
	for _, present := range []string{openOwnID, "Send the offer to Müller GmbH",
		"due 2026-10-28", "in Website relaunch", "assigned to Lisa Berg", "Second draft. With prices.",
		"Ask [person] about Müller"} {
		if !strings.Contains(material.Content, present) {
			t.Errorf("the material lacks %q:\n%s", present, material.Content)
		}
	}
	if len(searched) != 2 {
		t.Fatalf("searched %d times, want the named collection and the words", len(searched))
	}
	if searched[0].String("container_id") != websiteID || searched[0]["filter"] == nil {
		t.Errorf("the named collection was not searched for its open entries: %v", searched[0])
	}
	if q := searched[1].String("q"); !strings.Contains(q, "Müller or this") || strings.Contains(q, "?") {
		t.Errorf("the words searched were %q", q)
	}
}

// A question that names nothing and whose words find nothing has nothing to be answered from.
func TestAQuestionThatFindsNothingHasNoMaterial(t *testing.T) {
	sources, actor := askSources(nil)
	material, err := sources.Material(context.Background(), actor, askQuestion(actor, ""))
	if err != nil {
		t.Fatalf("the material could not be read: %v", err)
	}
	if material.Content != "" {
		t.Errorf("a question that finds nothing has material:\n%s", material.Content)
	}
}

func TestNobodyElsesQuestionHasAMaterial(t *testing.T) {
	sources, actor := askSources(nil)
	question := askQuestion(actor, "Müller")
	question.TargetID = shared.ID(lisaID)
	if _, err := sources.Material(context.Background(), actor, question); err == nil {
		t.Error("a material was read for a question about another account")
	}
}

// The words travel as a search for any of them, with the grammar's own signs left out and bounded
// as a query is.
func TestTheQuestionsWordsAreSearchedAsAnyOfThem(t *testing.T) {
	for _, testCase := range []struct{ question, want string }{
		{`What is "open" for -Müller?`, "What or is or open or for or Müller"},
		{"this or that", "this or that"},
		{"", ""},
	} {
		if got := wordQuery(testCase.question); got != testCase.want {
			t.Errorf("%q searched as %q, want %q", testCase.question, got, testCase.want)
		}
	}
	if got := wordQuery(strings.Repeat("word ", 100)); len([]rune(got)) > maxAskedWordQuery {
		t.Errorf("a long question searched as %d characters", len([]rune(got)))
	}
}
