// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestKudosGoToWhoeverCompletedTheEntry(t *testing.T) {
	done := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	completed := work.WorkItem{ID: "i1", Completion: work.Completion{
		IsCompleted: true, CompletedAt: &done, CompletedBy: "anna",
	}}
	nameless := work.WorkItem{ID: "i1", Completion: work.Completion{IsCompleted: true, CompletedAt: &done}}

	for name, tc := range map[string]struct {
		item  work.WorkItem
		giver shared.ID
		to    shared.ID
		code  string
	}{
		"somebody else's completion": {item: completed, giver: "ben", to: "anna"},
		"one's own completion":       {item: completed, giver: "anna", code: work.CodeKudosOwnCompletion},
		"an open entry":              {item: work.WorkItem{ID: "i1"}, giver: "ben", code: work.CodeKudosNotCompleted},
		"a completion naming nobody": {item: nameless, giver: "ben", code: work.CodeKudosNotCompleted},
	} {
		t.Run(name, func(t *testing.T) {
			to, err := work.KudosTo(tc.item, tc.giver)
			if tc.code == "" {
				if err != nil || to != tc.to {
					t.Fatalf("KudosTo = %q, %v; want %q", to, err, tc.to)
				}
				return
			}
			if got := shared.AsError(err).DetailCode; got != tc.code {
				t.Fatalf("refused with %q, want %q", got, tc.code)
			}
		})
	}
}

func TestOwnKudosAreAValidationRefusalAndAnOpenEntryAConflict(t *testing.T) {
	// The two mean different things to a client: an open entry may be completed and then
	// acknowledged (409, the state), one's own completion never (422, the request).
	_, err := work.KudosTo(work.WorkItem{}, "ben")
	if shared.AsError(err).Category != shared.CategoryConflict {
		t.Errorf("an open entry is %v", shared.AsError(err).Category)
	}
	_, err = work.KudosTo(work.WorkItem{Completion: work.Completion{IsCompleted: true, CompletedBy: "ben"}}, "ben")
	if shared.AsError(err).Category != shared.CategoryValidation {
		t.Errorf("one's own completion is %v", shared.AsError(err).Category)
	}
}
