// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func answerSets() *AnswerSets {
	return &AnswerSets{
		Entries: []string{openOwnID, namesKimID},
		Places:  []AnswerPlace{{ID: websiteID, Type: work.ContainerCollection, Name: "Website relaunch"}},
		People:  []Option{{ID: askerID, Name: "Jérôme Winkel (you)"}, {ID: lisaID, Name: "Lisa Berg"}},
		Labels:  []Option{{ID: urgentID, Name: "Urgent (in Website relaunch)"}},
	}
}

// An answer names only what its material showed: everything else is dropped and counted, and a
// place keeps the type the material recorded (UC-AI-08, ai-first.md §2).
func TestAnAnswerIsNarrowedToWhatWasShown(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		text    string
		want    map[string]any
		dropped int
		ok      bool
	}{
		{"an answer with its sources and its filter",
			`{"answer": " Two offers are open. ", "source_ids": ["` + openOwnID + `", "` + namesKimID + `"],
			  "filter": {"place_id": "` + websiteID + `", "is_completed": false, "due": "next_7_days",
			             "assignee_id": "` + lisaID + `", "label_ids": ["` + urgentID + `"]}}`,
			map[string]any{"answer": "Two offers are open.", "source_ids": []any{openOwnID, namesKimID},
				"filter": map[string]any{"place_id": websiteID, "place_type": "COLLECTION", "is_completed": false,
					"due": "next_7_days", "assignee_id": lisaID, "label_ids": []any{urgentID}}},
			0, true},
		{"an entry not shown, and one cited twice",
			`{"answer": "One.", "source_ids": ["` + byKimID + `", "` + openOwnID + `", "` + openOwnID + `"]}`,
			map[string]any{"answer": "One.", "source_ids": []any{openOwnID}}, 2, true},
		{"a place, a person and a label not shown",
			`{"answer": "x", "filter": {"place_id": "` + gardenID + `", "place_type": "HUB",
			  "assignee_id": "` + kimID + `", "label_ids": ["` + laterID + `"]}}`,
			map[string]any{"answer": "x"}, 4, true},
		{"a filter of anything else",
			`{"answer": "x", "filter": {"due": "2026-10-30", "is_completed": "no", "expr": "1 = 1"}}`,
			map[string]any{"answer": "x"}, 3, true},
		{"no answer text", `{"answer": "  ", "source_ids": ["` + openOwnID + `"]}`, nil, 0, false},
		{"not an object", `I cannot tell.`, nil, 0, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload, dropped, ok := answerFrom(testCase.text, answerSets())
			if ok != testCase.ok || dropped != testCase.dropped || !reflect.DeepEqual(payload, testCase.want) {
				t.Errorf("got %v, %d, %v\nwant %v, %d, %v", payload, dropped, ok, testCase.want, testCase.dropped, testCase.ok)
			}
		})
	}
}

func TestAnAnswerIsBounded(t *testing.T) {
	ids := `"` + strings.Repeat(openOwnID+`", "`, 3) + namesKimID + `"`
	payload, _, ok := answerFrom(`{"answer": "`+strings.Repeat("a", maxAnswerText+10)+`", "source_ids": [`+ids+`]}`, answerSets())
	if !ok || len([]rune(payload["answer"].(string))) != maxAnswerText {
		t.Errorf("the answer was not cut to its bound: %v", ok)
	}
	if sources := payload["source_ids"].([]any); len(sources) != 2 {
		t.Errorf("sources %v", sources)
	}
	if _, _, ok := answerFrom(`{"answer": "x"}`, nil); ok {
		t.Error("an answer without its material's sets was read")
	}
}
