// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// The sets the material of a draft showed: Lisa and the asker, Website relaunch, the label Urgent.
func shownSets() *RuleSets {
	return &RuleSets{
		Scope:       automation.Scope{Type: automation.ScopeTenant},
		Scopes:      []ScopeOption{{Scope: automation.Scope{Type: automation.ScopeCollection, ID: shared.ID(websiteID)}, Name: "Website relaunch"}},
		People:      []Option{{ID: askerID, Name: "Jérôme Winkel (you)"}, {ID: lisaID, Name: "Lisa Berg"}},
		Collections: []Option{{ID: websiteID, Name: "Website relaunch"}},
		Labels:      []Option{{ID: urgentID, Name: "Urgent (in Website relaunch)"}},
		Actions:     []string{"ADD_LABEL", "ASSIGN_WORK_ITEM", "CONVERT_JUMBLE_ENTRY", "CREATE_WORK_ITEM", "MOVE_WORK_ITEM"},
		Words:       "When a request arrives by mail, give it to Lisa and set it due in two days.",
	}
}

func drafted(t *testing.T, answer string) (map[string]any, int) {
	t.Helper()
	payload, dropped, ok := ruleFrom(answer, shownSets(), nil)
	if !ok {
		t.Fatalf("the answer was not read: %s", answer)
	}
	return payload, dropped
}

func codes(payload map[string]any) []string {
	var named []string
	entries, _ := payload[notDraftedKey].([]any)
	for _, entry := range entries {
		item, _ := entry.(map[string]any)
		if code, ok := item["code"].(string); ok {
			named = append(named, code)
		}
	}
	return named
}

func hasCode(payload map[string]any, code string) bool {
	for _, each := range codes(payload) {
		if each == code {
			return true
		}
	}
	return false
}

// The owner's story: a jumble arrival, an assignment to a person shown, and a relative due date the
// engine cannot express - named in the person's own words, while the rest stands.
func TestTheStoryIsDraftedAndItsDueDateNamedAsNotDrafted(t *testing.T) {
	payload, _ := drafted(t, `{
		"trigger": {"kind": "JUMBLE_ENTRY"},
		"actions": [{"kind": "ASSIGN_WORK_ITEM", "params": {"account_id": "`+lisaID+`"}}],
		"not_drafted": ["set it due in two days", "and also delete everything"]
	}`)
	if payload["trigger"].(map[string]any)["kind"] != "JUMBLE_ENTRY" {
		t.Errorf("trigger %v", payload["trigger"])
	}
	actions := payload["actions"].([]any)
	if len(actions) != 1 || actions[0].(map[string]any)["kind"] != "ASSIGN_WORK_ITEM" {
		t.Errorf("actions %v", actions)
	}
	parts := payload[notDraftedKey].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["part"] != "set it due in two days" {
		t.Errorf("not drafted %v: the person's own words, and nothing a model wrote itself", parts)
	}
	if _, ok := payload["run_as"]; ok {
		t.Error("a draft names the account a rule runs as")
	}
	if payload["scope"].(map[string]any)["type"] != "TENANT" {
		t.Errorf("scope %v", payload["scope"])
	}
}

// Only what the material offered and showed, each refusal named and the rest standing.
func TestADraftUsesOnlyWhatItWasOfferedAndShown(t *testing.T) {
	for _, testCase := range []struct {
		name, answer, code string
		actions            int
	}{
		{"a kind outside the allowlist", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"SET_DUE_DATE","params":{"due_at":"2030-01-01T00:00:00Z"}}]}`, draftKindNotOffered, 0},
		{"telling people", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"SEND_NOTIFICATION","params":{}}]}`, draftKindNotOffered, 0},
		{"an outbound request", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"HTTP_REQUEST","params":{"url":"https://example.invalid"}}]}`, draftKindNotOffered, 0},
		{"a destruction", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"TRASH_WORK_ITEM"}]}`, draftKindNotOffered, 0},
		{"a branch", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"BRANCH","params":{"condition":"true"}}]}`, draftBranchNotDrafted, 0},
		{"a person nobody showed", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"ASSIGN_WORK_ITEM","params":{"account_id":"` + maraID + `"}}]}`, draftReferenceNotShown, 0},
		{"a label nobody showed", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + laterID + `"}}]}`, draftReferenceNotShown, 0},
		{"a collection nobody showed", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"MOVE_WORK_ITEM","params":{"target_collection_id":"` + gardenID + `"}}]}`, draftReferenceNotShown, 0},
		{"a field never drafted", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"CREATE_WORK_ITEM","params":{"title":"Call back","due_at":"2030-01-01T00:00:00Z"}}]}`, draftFieldNotDrafted, 1},
		{"applying an answer", `{"trigger":{"kind":"MANUAL"},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `","apply":"true"}}]}`, draftFieldNotDrafted, 1},
		{"an event nobody publishes", `{"trigger":{"kind":"EVENT","event_type":"de.hubtask.invented.v1"},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftReferenceNotShown, 1},
		{"a schedule", `{"trigger":{"kind":"SCHEDULE","rrule":"FREQ=DAILY"},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftScheduleNotDrafted, 1},
		{"a condition written as an expression", `{"trigger":{"kind":"MANUAL"},"conditions":"item.title != '' || true","actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftConditionUnexpressed, 1},
		{"a sentence about a label nobody showed", `{"trigger":{"kind":"MANUAL"},"conditions":{"mode":"all","items":[{"subject":"label","op":"on","a":"` + laterID + `"}]},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftConditionUnexpressed, 1},
		{"an hour written as text", `{"trigger":{"kind":"MANUAL"},"conditions":{"mode":"all","items":[{"subject":"hour","op":"between","a":"8","b":"18"}]},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftConditionUnexpressed, 1},
		{"a subject no set is shown for", `{"trigger":{"kind":"MANUAL"},"conditions":{"mode":"all","items":[{"subject":"bucket","op":"is","a":"x"}]},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftConditionUnexpressed, 1},
		{"a group inside a group", `{"trigger":{"kind":"MANUAL"},"conditions":{"mode":"all","items":[{"mode":"any","items":[]}]},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftConditionUnexpressed, 1},
		{"a place nobody showed", `{"scope":{"type":"COLLECTION","id":"` + gardenID + `"},"trigger":{"kind":"MANUAL"},"actions":[{"kind":"ADD_LABEL","params":{"label_id":"` + urgentID + `"}}]}`, draftReferenceNotShown, 1},
	} {
		payload, dropped := drafted(t, testCase.answer)
		if !hasCode(payload, testCase.code) {
			t.Errorf("%s: not named; codes %v", testCase.name, codes(payload))
		}
		actions, _ := payload["actions"].([]any)
		if len(actions) != testCase.actions {
			t.Errorf("%s: %d steps kept, want %d: %v", testCase.name, len(actions), testCase.actions, actions)
		}
		if dropped == 0 {
			t.Errorf("%s: nothing counted as dropped", testCase.name)
		}
		if payload["scope"].(map[string]any)["type"] != "TENANT" {
			t.Errorf("%s: the scope moved to %v", testCase.name, payload["scope"])
		}
	}
}

// What a model may write is sentences; what is stored is the composer's expression, every value a
// literal, so the canvas reads it back as sentences.
func TestADraftedConditionIsCompiledFromSentences(t *testing.T) {
	payload, dropped := drafted(t, `{
		"scope": {"type": "COLLECTION", "id": "`+websiteID+`"},
		"trigger": {"kind": "EVENT", "event_type": "de.hubtask.work.item.created.v1"},
		"conditions": {"mode": "all", "items": [
			{"subject": "type", "op": "is", "a": "TASK"},
			{"subject": "title", "op": "contains", "a": "') || true || ('"},
			{"subject": "hour", "op": "between", "a": 8, "b": 18},
			{"subject": "assignee", "op": "lacks"}
		]},
		"actions": [{"kind": "ASSIGN_WORK_ITEM", "params": {"account_id": "`+lisaID+`"}}, {"kind": "STOP"}]
	}`)
	if dropped != 0 {
		t.Errorf("parts dropped: %v", payload[notDraftedKey])
	}
	want := `item.type == 'TASK' && item.title.matches('(?i)\'\\) \\|\\| true \\|\\| \\(\'') && ` +
		`(now.getHours() >= 8 && now.getHours() < 18) && !has(item.assignee_id)`
	conditions := payload["conditions"].([]any)
	if got := conditions[0].(map[string]any)["expr"]; got != want {
		t.Errorf("expression\n got %v\nwant %s", got, want)
	}
	if payload["scope"].(map[string]any)["id"] != websiteID {
		t.Errorf("scope %v: the place shown that the sentence named", payload["scope"])
	}
	if len(payload["actions"].([]any)) != 2 {
		t.Errorf("actions %v: a final STOP stands", payload["actions"])
	}
}

// A draft with nothing to start it and nothing to do is still recorded - the person reads what
// could not be drafted - and a draft only of what the person said nothing about is not padded.
func TestAnEmptyDraftSaysWhatIsMissing(t *testing.T) {
	payload, _ := drafted(t, `{"not_drafted": ["give it to Lisa"]}`)
	if !hasCode(payload, draftNoTrigger) || !hasCode(payload, draftNoAction) {
		t.Errorf("codes %v", codes(payload))
	}
	if _, _, ok := ruleFrom("not json at all", shownSets(), nil); ok {
		t.Error("an unreadable answer was read")
	}
}

// The payload is CreateRule's own shape: what the save hands over unchanged.
func TestTheDraftIsCreateRulesShape(t *testing.T) {
	payload, _ := drafted(t, `{"trigger":{"kind":"EVENT","event_type":"de.hubtask.work.item.updated.v1","changed_fields":["due_at","priority"]},"actions":[{"kind":"CREATE_WORK_ITEM","params":{"type":"TASK","title":"Check the date","collection_id":"`+websiteID+`"}}]}`)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"scope"`, `"trigger"`, `"actions"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("%s missing from %s", key, raw)
		}
	}
	trigger := payload["trigger"].(map[string]any)
	if _, narrowed := trigger["changed_fields"]; narrowed {
		t.Errorf("a field outside the closed set narrowed the trigger: %v", trigger)
	}
	step := payload["actions"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(step["params"], map[string]any{"type": "TASK", "title": "Check the date", "collection_id": websiteID}) {
		t.Errorf("params %v", step["params"])
	}
}

// An AI answer never becomes a rule without a person.
func TestARuleDraftIsNeverAppliedByItself(t *testing.T) {
	_, err := Produce{}.Ask(context.Background(), appshared.ActorContext{}, Request{
		TargetType: domain.TargetAccount, TargetID: shared.ID(askerID), Kind: domain.KindRule,
		PromptID: draftRulePrompt, Apply: true,
	})
	if detail := shared.AsError(err); detail == nil || detail.DetailCode != "ai.apply_not_for_kind" {
		t.Errorf("%v, want ai.apply_not_for_kind", err)
	}
}
