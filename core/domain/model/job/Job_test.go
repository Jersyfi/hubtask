// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package job_test

import (
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestStateOfTranslatesEveryStoredState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		stored string
		want   domain.State
	}{
		{domain.StoredPending, domain.StateQueued},
		{domain.StoredRunning, domain.StateRunning},
		{domain.StoredSucceeded, domain.StateSucceeded},
		{domain.StoredFailed, domain.StateFailed},
		// The one that is not a rename: the dead letter is where an operator finds the row, and
		// the caller is told the only thing they can act on.
		{domain.StoredDeadLetter, domain.StateFailed},
		{domain.StoredCancelled, domain.StateCancelled},
	}

	for _, c := range cases {
		t.Run(c.stored, func(t *testing.T) {
			t.Parallel()
			got, err := domain.StateOf(c.stored)
			if err != nil {
				t.Fatalf("StateOf(%q): %v", c.stored, err)
			}
			if got != c.want {
				t.Fatalf("StateOf(%q) = %q, want %q", c.stored, got, c.want)
			}
		})
	}
}

// The six the table allows are the six above. A seventh would mean a version wrote a state this
// one does not know, and answering QUEUED for it would tell a client to keep polling something
// that is over.
func TestStateOfRefusesAnUnknownState(t *testing.T) {
	t.Parallel()

	if _, err := domain.StateOf("SLEEPING"); !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("StateOf(SLEEPING) = %v, want an internal error", err)
	}
	if _, err := domain.StateOf(""); !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("StateOf(empty) = %v, want an internal error", err)
	}
}

func TestTerminalStates(t *testing.T) {
	t.Parallel()

	cases := map[domain.State]bool{
		domain.StateQueued:    false,
		domain.StateRunning:   false,
		domain.StateSucceeded: true,
		domain.StateFailed:    true,
		domain.StateCancelled: true,
	}

	for state, want := range cases {
		if got := state.IsTerminal(); got != want {
			t.Fatalf("%q.IsTerminal() = %v, want %v", state, got, want)
		}
	}
}

func TestCancellableRefusesATerminalJob(t *testing.T) {
	t.Parallel()

	id := shared.MustParseID("018f4b1e-0000-7000-8000-00000000000a")

	for _, state := range []domain.State{domain.StateQueued, domain.StateRunning} {
		if err := (domain.Job{ID: id, State: state}).Cancellable(); err != nil {
			t.Fatalf("%q: Cancellable = %v, want nil", state, err)
		}
	}

	for _, state := range []domain.State{
		domain.StateSucceeded, domain.StateFailed, domain.StateCancelled,
	} {
		err := (domain.Job{ID: id, State: state}).Cancellable()
		if !errors.Is(err, shared.ErrConflict) {
			t.Fatalf("%q: Cancellable = %v, want a conflict", state, err)
		}
		// The refusal names the state it refused on: "already cancelled" and "already succeeded"
		// are different things to whoever asked.
		if got := shared.AsError(err).Params["status"]; got != state.String() {
			t.Fatalf("%q: the conflict named status %q", state, got)
		}
	}
}
