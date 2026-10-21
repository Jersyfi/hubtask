// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func sentence(subject, op, a, b string) automation.ConditionNode {
	return automation.ConditionNode{Sentence: &automation.Sentence{Subject: subject, Op: op, A: a, B: b}}
}

func group(mode string, items ...automation.ConditionNode) automation.ConditionNode {
	return automation.ConditionNode{Group: &automation.SentenceGroup{Mode: mode, Items: items}}
}

// The shapes themselves are held to the editor's by the shared vectors
// (test/architecture/conditionsentences_test.go); this is what the server adds - the bounds.
func TestASentenceIsCompiledOnlyInsideItsBounds(t *testing.T) {
	for _, testCase := range []struct {
		name string
		node automation.ConditionNode
		want string
	}{
		{"a hostile value stays one literal", sentence("title", "contains", "') || true || ('", ""),
			`item.title.matches('(?i)\'\\) \\|\\| true \\|\\| \\(\'')`},
		{"the hour is a range inside the day", sentence("hour", "between", "8", "18"),
			"now.getHours() >= 8 && now.getHours() < 18"},
		{"a group joins and parenthesises", group(automation.ModeNone, sentence("due", "past", "", ""), sentence("completed", "yes", "", "")),
			"!((has(item.due_at) && item.due_at < now) || item.completed == true)"},
		{"an empty group is no condition", group(automation.ModeAll), ""},
	} {
		got, err := automation.CompileCondition(testCase.node)
		if err != nil || got != testCase.want {
			t.Errorf("%s: %q, %v; want %q", testCase.name, got, err, testCase.want)
		}
	}

	for _, refused := range []struct {
		name string
		node automation.ConditionNode
	}{
		{"a line break", sentence("notes", "contains", "a\nb", "")},
		{"an empty value", sentence("label", "on", "", "")},
		{"a value too long", sentence("title", "contains", string(make([]rune, automation.MaxSentenceText+1)), "")},
		{"an hour backwards", sentence("hour", "between", "18", "8")},
		{"a depth too deep", sentence("depth", "is", "5", "")},
		{"days that are not digits", sentence("due", "within", "+3", "")},
		{"an unknown operator", sentence("type", "contains", "TASK", "")},
		{"an unknown mode", group("some", sentence("due", "has", "", ""))},
		{"neither a sentence nor a group", automation.ConditionNode{}},
		{"a refusal inside a group", group(automation.ModeAll, sentence("depth", "is", "9", ""))},
	} {
		_, err := automation.CompileCondition(refused.node)
		if detail := shared.AsError(err); detail == nil || detail.DetailCode != "automation.sentence_invalid" {
			t.Errorf("%s: %v, want automation.sentence_invalid", refused.name, err)
		}
	}
}

// Every operator of every subject compiles with values of the kind it takes, and to the shape the
// editor writes for it: the expression names what the subject reads.
func TestEveryOperatorOfEverySubjectCompiles(t *testing.T) {
	reads := map[string]string{
		"type": "item.type", "title": "item.title", "notes": "item.notes", "completed": "item.completed",
		"archived": "item.archived", "due": "item.due_at", "label": "item.labels", "assignee": "item.assignee_id",
		"bucket": "item.bucket_id", "parent": "item.parent_id", "depth": "item.depth", "actor": "actor.id",
		"hour": "now.getHours()", "field": "item.custom_fields",
	}
	subjects := automation.SentenceSubjects()
	if len(subjects) != len(reads) {
		t.Fatalf("%d subjects, the table knows %d", len(subjects), len(reads))
	}
	for _, subject := range subjects {
		for _, op := range automation.SentenceOperators(subject) {
			s := automation.Sentence{Subject: subject, Op: op}
			switch automation.SentenceTakes(subject, op) {
			case "value":
				s.A = "x"
			case "key_value":
				s.A, s.B = "priority", "High"
			case "number", "days":
				s.A = "2"
			case "range":
				s.A, s.B = "8", "18"
			}
			expr, err := automation.CompileSentence(s)
			if err != nil || !strings.Contains(expr, reads[subject]) {
				t.Errorf("%s %s: %q, %v", subject, op, expr, err)
			}
		}
	}
	if len(automation.SentenceOperators("priority")) != 0 {
		t.Error("an unknown subject has operators")
	}
	for _, refused := range []automation.Sentence{
		{Subject: "field", Op: "is", A: "priority"},
		{Subject: "hour", Op: "between", A: "8"},
		{Subject: "depth", Op: "is", A: "2", B: "3"},
		{Subject: "label", Op: "on", A: "x", B: "y"},
		{Subject: "due", Op: "within", A: "1234"},
	} {
		if _, err := automation.CompileSentence(refused); err == nil {
			t.Errorf("%+v compiled", refused)
		}
	}
}
