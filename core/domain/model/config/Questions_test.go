// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func escalationDocument() Document {
	return Document{
		"rules": Section{
			"p1_escalation": Block{
				"name": "P1",
				"actions": []any{map[string]any{
					"kind": "SET_CUSTOM_FIELD", "params": map[string]any{"key": "responding_group", "value": "answer:first_group"},
				}},
			},
			"acknowledge": Block{"name": "Acknowledge"},
		},
		"settings": Section{"desk.greeting": Block{"value": "answer:greeting"}},
	}
}

var deskQuestions = []Question{
	{Key: "first_group", Kind: QuestionGroup},
	{Key: "greeting", Kind: QuestionText, Default: "Hello"},
	{Key: "tone", Kind: QuestionChoice, Options: []string{"formal", "casual"}, Required: true},
}

// A template's questions are answered when it is applied (configuration.md §6): an answer takes the
// place of its mark, a question left open takes its default, and a block naming a question nobody
// answered and nothing defaults is left out rather than applied half-made.
func TestAnswersTakeThePlaceOfTheirMarks(t *testing.T) {
	group := "01936f2a-7c1e-7000-8000-000000000009"
	answered, err := Answer(deskQuestions, escalationDocument(), map[string]string{"first_group": group, "tone": "casual"})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	action := answered["rules"]["p1_escalation"]["actions"].([]any)[0].(map[string]any)
	if action["params"].(map[string]any)["value"] != group {
		t.Errorf("the escalation names %v", action)
	}
	if answered["settings"]["desk.greeting"]["value"] != "Hello" {
		t.Errorf("an open question with a default is %v", answered["settings"])
	}
	if escalationDocument()["rules"]["p1_escalation"]["actions"].([]any)[0].(map[string]any)["params"].(map[string]any)["value"] != "answer:first_group" {
		t.Error("answering changed the template")
	}

	open, err := Answer(deskQuestions, escalationDocument(), map[string]string{"tone": "formal"})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if _, held := open["rules"]["p1_escalation"]; held {
		t.Error("a block naming an unanswered question was kept")
	}
	if _, held := open["rules"]["acknowledge"]; !held {
		t.Error("a block naming no question was left out")
	}
}

func TestAWrongAnswerIsRefusedByName(t *testing.T) {
	cases := map[string]struct {
		answers map[string]string
		path    string
		code    string
	}{
		"a question the template does not ask": {map[string]string{"tone": "formal", "colour": "red"}, "/answers/colour", "config.answer_unknown"},
		"a required question left open":        {map[string]string{}, "/answers/tone", "config.answer_required"},
		"a group that is no identifier":        {map[string]string{"tone": "formal", "first_group": "Second line"}, "/answers/first_group", "config.answer_not_a_group"},
		"a choice not offered":                 {map[string]string{"tone": "loud"}, "/answers/tone", "config.answer_not_an_option"},
		"an empty text":                        {map[string]string{"tone": "formal", "greeting": "  "}, "/answers/greeting", "config.answer_required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Answer(deskQuestions, escalationDocument(), tc.answers)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want a refusal", err)
			}
			var coded *shared.Error
			errors.As(err, &coded)
			var found bool
			for _, field := range coded.Fields {
				if field.Path == tc.path && field.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Errorf("refusals %v, want %s at %s", coded.Fields, tc.code, tc.path)
			}
		})
	}
}

func TestAQuestionIsDeclaredWhole(t *testing.T) {
	bad := [][]Question{
		{{Key: "Group", Kind: QuestionGroup}},
		{{Key: "g", Kind: "COLOUR"}},
		{{Key: "c", Kind: QuestionChoice}},
		{{Key: "g", Kind: QuestionGroup}, {Key: "g", Kind: QuestionText}},
		{{Key: "c", Kind: QuestionChoice, Options: []string{"a"}, Default: "b"}},
	}
	for _, questions := range bad {
		if err := ValidQuestions(questions); err == nil {
			t.Errorf("%v was accepted", questions)
		}
	}
	if err := ValidQuestions(deskQuestions); err != nil {
		t.Errorf("the desk's questions: %v", err)
	}
}
