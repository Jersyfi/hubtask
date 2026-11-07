// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func TestAnEntrysPresenceSitsAtItsCollectionAndIsNeverWritten(t *testing.T) {
	h := newLifecycleHarness()
	locator := PresenceLocator{Items: h.items, Containers: h.containers}

	place, err := locator.Locate(t.Context(), itemActor(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !place.PresenceOnly || place.Read != service.PermissionRead {
		t.Errorf("the place is %+v, want presence only, read with READ", place)
	}
	if len(place.Path) != 3 || place.Path[2].ID != collectionID {
		t.Errorf("the path is %+v, want the tenant, the hub and the collection", place.Path)
	}
	if place.On.ID != taskID || place.On.Does != service.ItemRead {
		t.Errorf("the request names %+v, want the entry read", place.On)
	}
}

func TestATrashedEntryHasNoPresence(t *testing.T) {
	h := newLifecycleHarness()
	trashed := h.items.stored[taskID]
	at := time.Now()
	trashed.DeletedAt = &at
	h.items.stored[taskID] = trashed

	_, err := PresenceLocator{Items: h.items, Containers: h.containers}.Locate(t.Context(), itemActor(), taskID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a trashed entry's presence answered %v, want not found", err)
	}
}
