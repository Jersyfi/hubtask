// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/automation"
)

// The server compiles a model's condition sentences with a twin of the rule editor's composer, and
// the editor draws them as sentences only where it reads back exactly the shapes it writes. The two
// compilers are held together by one vector file the editor's own test reads too
// (apps/webapp/src/lib/automation/model.test.ts): a difference of one byte is a condition the
// canvas shows as raw text, or worse, as a different sentence.
//
// This test lives here rather than beside the compiler because the vectors are the editor's file,
// and core knows nothing about a frontend (rule 14).
func TestTheServerCompilesConditionSentencesExactlyAsTheEditorDoes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "apps", "webapp", "src", "lib", "automation", "sentences.golden.json"))
	if err != nil {
		t.Fatalf("the vectors are missing: %v", err)
	}
	var vectors struct {
		Compiled []struct {
			Name string          `json:"name"`
			Node json.RawMessage `json:"node"`
			Expr string          `json:"expr"`
		} `json:"compiled"`
		Refused []struct {
			Name string          `json:"name"`
			Node json.RawMessage `json:"node"`
		} `json:"refused"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("the vectors do not read: %v", err)
	}
	if len(vectors.Compiled) == 0 || len(vectors.Refused) == 0 {
		t.Fatal("the vector file holds nothing to compare")
	}

	for _, vector := range vectors.Compiled {
		node, err := conditionNode(vector.Node)
		if err != nil {
			t.Errorf("%s: %v", vector.Name, err)
			continue
		}
		got, err := automation.CompileCondition(node)
		if err != nil {
			t.Errorf("%s: the server refused what the editor compiles: %v", vector.Name, err)
			continue
		}
		if got != vector.Expr {
			t.Errorf("%s:\n server %s\n editor %s", vector.Name, got, vector.Expr)
		}
	}
	for _, vector := range vectors.Refused {
		node, err := conditionNode(vector.Node)
		if err != nil {
			continue
		}
		if got, err := automation.CompileCondition(node); err == nil {
			t.Errorf("%s: compiled to %q, and should have been refused", vector.Name, got)
		}
	}
}

// conditionNode reads the editor's shape: a sentence {subject, op, a?, b?} or a group {mode, items}.
func conditionNode(raw json.RawMessage) (automation.ConditionNode, error) {
	var shape struct {
		Subject *string           `json:"subject"`
		Op      string            `json:"op"`
		A       string            `json:"a"`
		B       string            `json:"b"`
		Mode    *string           `json:"mode"`
		Items   []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return automation.ConditionNode{}, err
	}
	if shape.Subject != nil {
		return automation.ConditionNode{Sentence: &automation.Sentence{
			Subject: *shape.Subject, Op: shape.Op, A: shape.A, B: shape.B,
		}}, nil
	}
	group := &automation.SentenceGroup{}
	if shape.Mode != nil {
		group.Mode = *shape.Mode
	}
	for _, item := range shape.Items {
		node, err := conditionNode(item)
		if err != nil {
			return automation.ConditionNode{}, err
		}
		group.Items = append(group.Items, node)
	}
	return automation.ConditionNode{Group: group}, nil
}
