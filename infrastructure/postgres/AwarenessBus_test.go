// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type countedOutcomes []string

func (c *countedOutcomes) AwarenessRelayed(_ context.Context, outcome string) {
	*c = append(*c, outcome)
}

// refusingWork answers err for every transaction without running it.
type refusingWork struct {
	persistence.UnitOfWork
	err error
}

func (w refusingWork) Within(context.Context, persistence.Scope, func(context.Context) error) error {
	return w.err
}

// Awareness is best effort, so a message that does not cross to the other replicas is never the
// connection's error - which is why each outcome is counted: an operator learns from the counter
// that co-editors on different replicas stopped seeing each other (offline-sync.md §13).
func TestEveryAwarenessPublicationIsCountedByOutcome(t *testing.T) {
	var counted countedOutcomes
	down := errors.New("connection refused")
	bus := AwarenessBus{Work: refusingWork{err: down}, Listener: &ChangeListener{origin: "a1"}, Signals: &counted}
	tenant, document := shared.ID("0192a7c0-0000-7000-8000-000000000001"), shared.ID("0192a7c0-0000-7000-8000-000000000002")

	if err := bus.Publish(context.Background(), tenant, "page", document, bytes.Repeat([]byte{7}, 6000)); err != nil {
		t.Fatalf("a state too large for a notification is not an error: %v", err)
	}
	if err := bus.Publish(context.Background(), tenant, "page", document, []byte("cursor")); !errors.Is(err, down) {
		t.Fatalf("a failed publication answers %v", err)
	}
	bus.Work = refusingWork{}
	if err := bus.Publish(context.Background(), tenant, "page", document, []byte("cursor")); err != nil {
		t.Fatal(err)
	}
	if want := []string{"too_large", "failed", "sent"}; len(counted) != 3 || counted[0] != want[0] || counted[1] != want[1] || counted[2] != want[2] {
		t.Fatalf("counted %v, want %v", counted, want)
	}
}
