// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package persistence

import (
	"context"
	"testing"
)

func TestHeldWorkRunsWhenItsHolderSaysAndNotBefore(t *testing.T) {
	ctx, held := HoldAfterwards(t.Context())
	var ran []string
	After(ctx, func(context.Context) { ran = append(ran, "first") })

	// A nested transaction holds nothing of its own: the outer holder runs its work.
	inner, nested := HoldAfterwards(ctx)
	if nested != nil {
		t.Fatal("a nested transaction took a holder of its own")
	}
	After(inner, func(context.Context) { ran = append(ran, "second") })
	if len(ran) != 0 {
		t.Fatalf("held work ran early: %v", ran)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	var detached bool
	After(ctx, func(ctx context.Context) { detached = ctx.Err() == nil })
	held.Run(cancelled)
	if len(ran) != 2 || ran[0] != "first" || ran[1] != "second" || !detached {
		t.Errorf("ran %v, detached %v", ran, detached)
	}

	held.Run(ctx)
	if len(ran) != 2 {
		t.Errorf("held work ran twice: %v", ran)
	}
}

func TestWorkWithoutAHolderRunsAtOnce(t *testing.T) {
	ran := false
	After(t.Context(), func(context.Context) { ran = true })
	if !ran {
		t.Error("work without a holder waited for nobody")
	}
	var nobody *Afterwards
	nobody.Run(t.Context())
}

// Work that runs afterwards and opens a transaction of its own holds its own afterwards again,
// rather than into the emptied holder.
func TestHeldWorkOpensItsOwnHolder(t *testing.T) {
	ctx, held := HoldAfterwards(t.Context())
	var again *Afterwards
	After(ctx, func(ctx context.Context) {
		_, again = HoldAfterwards(ctx)
	})
	held.Run(ctx)
	if again == nil {
		t.Error("work run afterwards could not hold work of its own")
	}
}
