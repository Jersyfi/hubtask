// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"
	"time"
)

// A completion counts, a repeat does not, and a reopen keeps the count (UC-WRK-25 check 4,
// UC-WRK-04 check 5).
func TestTheCompletionCountMovesOnlyWithACompletion(t *testing.T) {
	item, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	done := item.Completed(itemActor, at)
	again := done.Completed(itemActor, at.Add(time.Minute))
	reopened := again.Reopened(at.Add(2 * time.Minute))
	twice := reopened.Completed(itemActor, at.Add(3*time.Minute))

	for name, c := range map[string]struct{ got, want int }{
		"new": {item.CompletionCount, 0}, "completed": {done.CompletionCount, 1},
		"completed again": {again.CompletionCount, 1}, "reopened": {reopened.CompletionCount, 1},
		"completed twice": {twice.CompletionCount, 2},
	} {
		if c.got != c.want {
			t.Errorf("%s: count %d, want %d", name, c.got, c.want)
		}
	}
	if reopened.Completion.IsCompleted || reopened.Completion.CompletedAt != nil {
		t.Errorf("the reopen kept the completion: %+v", reopened.Completion)
	}
}
