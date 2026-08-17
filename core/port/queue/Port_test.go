// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package queue

import "testing"

// The one decision the port makes on its own: whether a failure now is the last one. It sits here
// rather than in the runner because the dead letter and the retry read the same answer, and two
// readings of "have the attempts run out" that disagree is how a job is retried forever.
func TestTheLastAttemptIsTheOneThatUsesUpTheBudget(t *testing.T) {
	cases := []struct {
		name        string
		attempts    int
		maxAttempts int
		want        bool
	}{
		{"the first of eight", 1, 8, false},
		{"the seventh of eight", 7, 8, false},
		{"the eighth of eight", 8, 8, true},
		// A job whose budget was lowered while it was waiting is over its allowance, not owed
		// another round.
		{"beyond the budget", 9, 8, true},
		// One attempt means no retry, which is the honest reading of a job that may not be
		// repeated at all.
		{"a single attempt", 1, 1, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			job := Job{Attempts: c.attempts, MaxAttempts: c.maxAttempts}
			if got := job.LastAttempt(); got != c.want {
				t.Errorf("LastAttempt() = %v for attempt %d of %d, want %v",
					got, c.attempts, c.maxAttempts, c.want)
			}
		})
	}
}
