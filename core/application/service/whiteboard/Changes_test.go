// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changeLog struct{ changes []changelog.Change }

func (l *changeLog) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type hlcs struct{ reading shared.HLC }

func (h *hlcs) Next() shared.HLC {
	next, _ := h.reading.Tick(start, "server")
	h.reading = next
	return next
}

// UC-CNV-01 check 9: every write of a board tells the change log the board as it now stands, its
// scene included and filed under its hub, so a device reads it offline; a trash says the board is
// in the trash.
func TestEveryWriteOfABoardReachesTheChangeLog(t *testing.T) {
	s, _, _ := studio(gate{})
	log := &changeLog{}
	s.Changes, s.HLC = log, &hlcs{}
	ctx := context.Background()
	out, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"hub_id": hub.String(), "title": "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(string)
	if _, err := (UpdateWhiteboard{Studio: s}).Descriptor().Handler.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": id, "elements": sent()}); err != nil {
		t.Fatal(err)
	}
	if _, err := (TrashWhiteboard{Studio: s}).Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"whiteboard_id": id}); err != nil {
		t.Fatal(err)
	}
	if len(log.changes) != 3 {
		t.Fatalf("%d changes after making, drawing and trashing a board, want 3", len(log.changes))
	}
	drawn := log.changes[1]
	if drawn.Entity != "whiteboard" || drawn.EntityID.String() != id || drawn.ContainerID != hub || drawn.Op != changelog.Upsert ||
		drawn.Payload["element_count"] != 5 || len(drawn.Payload["elements"].([]usecase.Output)) != 5 {
		t.Errorf("the drawing was recorded as %+v", drawn)
	}
	if log.changes[2].Payload["trashed_at"] == nil {
		t.Errorf("the trash was recorded as %+v", log.changes[2])
	}
}
