// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type crossingsFake struct {
	admitted []string
	refused  []string
	limits   []int64
	who      []shared.ID
}

func (c *crossingsFake) Admitted(_ context.Context, quota string) error {
	c.admitted = append(c.admitted, quota)
	return nil
}

func (c *crossingsFake) Refused(_ context.Context, workspace shared.ID, quota string, limit int64) {
	c.refused, c.limits, c.who = append(c.refused, quota), append(c.limits, limit), append(c.who, workspace)
}

// The guard tells the crossings which creates it admitted and which it refused at which limit, for
// every capacity quota, and an unlimited quota neither - it can be reached by nothing.
func TestTheGuardTellsWhatItAdmittedAndRefused(t *testing.T) {
	store := &storeFake{overrides: repository.Overrides{Items: ceiling(10), MediaBytes: ceiling(100)}}
	usage := &usageFake{items: 9, media: 95}
	crossings := &crossingsFake{}
	guard := guardWith(store, usage, &signalsFake{})
	guard.Crossings = crossings

	if err := guard.Items(t.Context(), "0192f000-0000-7000-8000-00000000000a", 1); err != nil {
		t.Fatal(err)
	}
	usage.items = 10
	if err := guard.Items(t.Context(), "0192f000-0000-7000-8000-00000000000a", 1); err == nil {
		t.Fatal("the wall let a create through")
	}
	if err := guard.MediaBytes(t.Context(), "0192f000-0000-7000-8000-00000000000a", 10); err == nil {
		t.Fatal("storage passed its ceiling")
	}
	if len(crossings.admitted) != 1 || crossings.admitted[0] != Items {
		t.Errorf("admitted %v", crossings.admitted)
	}
	if len(crossings.refused) != 2 || crossings.refused[0] != Items || crossings.limits[0] != 10 ||
		crossings.refused[1] != MediaBytes || crossings.limits[1] != 100 ||
		crossings.who[0] != "0192f000-0000-7000-8000-00000000000a" {
		t.Errorf("refused %v at %v for %v", crossings.refused, crossings.limits, crossings.who)
	}

	store.overrides = repository.Overrides{}
	before := len(crossings.admitted) + len(crossings.refused)
	if err := guard.Items(t.Context(), "0192f000-0000-7000-8000-00000000000a", 1); err != nil {
		t.Fatal(err)
	}
	if len(crossings.admitted)+len(crossings.refused) != before {
		t.Error("an unlimited quota told the crossings something")
	}
}
