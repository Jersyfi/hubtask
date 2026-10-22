// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"slices"
	"sort"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
)

// What a model may put into a rule it drafts from a sentence is an allowlist of entry-level kinds,
// each field classified (UC-AUT-10 check 2). A table in code goes stale the day a use case gains a
// field: that field would be one nobody decided a model may set, and the narrowing would pass it
// on as unclassified. So every kind in the table is one the catalogue serves, none is destructive,
// and every field its use case declares for a rule is either drafted or named as never drafted.
func TestEveryDraftableKindIsServedSafeAndFullyClassified(t *testing.T) {
	byAction := map[string]bool{}
	destructive := map[string]bool{}
	fields := map[string][]string{}
	for _, descriptor := range usecases.Descriptors() {
		kind := descriptor.AutomationAction()
		byAction[kind] = true
		destructive[kind] = descriptor.Destructive
		for _, field := range descriptor.Input {
			if !field.CallerOnly {
				fields[kind] = append(fields[kind], field.Name)
			}
		}
	}

	table := suggestion.DraftableActions()
	if len(table) == 0 {
		t.Fatal("a draft may do nothing at all")
	}
	for kind, how := range table {
		if !byAction[kind] {
			t.Errorf("%s is draftable and no use case serves it", kind)
			continue
		}
		if destructive[kind] {
			t.Errorf("%s is draftable and destructive", kind)
		}
		classified := append(append([]string(nil), how.Drafted...), how.Never...)
		for _, field := range fields[kind] {
			if !slices.Contains(classified, field) {
				t.Errorf("%s declares %s for a rule, and the draft table neither drafts it nor "+
					"says it never does: classify it in draftableActions "+
					"(core/application/service/suggestion/Suggestions.go)", kind, field)
			}
		}
		for _, field := range how.Drafted {
			if !slices.Contains(fields[kind], field) {
				t.Errorf("%s drafts %s, which its use case does not declare", kind, field)
			}
		}
	}

	kinds := make([]string, 0, len(table))
	for kind := range table {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, never := range []string{"HTTP_REQUEST", "SEND_WEBHOOK", "DESCRIBE_RULE", "TRASH_WORK_ITEM"} {
		if slices.Contains(kinds, never) {
			t.Errorf("%s is draftable: an address, a destruction or a question to AI", never)
		}
	}
}
