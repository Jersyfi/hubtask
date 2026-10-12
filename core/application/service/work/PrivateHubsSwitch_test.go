// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-16 check 7: with *Private hubs allowed* off, nobody makes a new private hub - not by
// creating one, not by marking one - and every private hub already there stays as it is: it is
// shared again by its owner as before.
func TestPrivateHubsSwitchedOffStopsNewOnes(t *testing.T) {
	t.Run("creating", func(t *testing.T) {
		_, h, grants := privateHarness(workspaceMember)
		h.handler.PrivateHubs = privateHubsSwitch{off: true}
		registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
		if err != nil {
			t.Fatal(err)
		}
		_, err = registry.Invoke(context.Background(), CreateContainerName, actor(),
			usecase.Input{"type": "HUB", "name": "Diary", "private": true})
		if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "containers.private_hubs_off" {
			t.Fatalf("error %v, want containers.private_hubs_off", err)
		}
		if len(h.containers.inserted) != 0 || len(grants.granted) != 0 {
			t.Error("a hub or a grant was written")
		}
	})

	t.Run("marking", func(t *testing.T) {
		h := newPrivacyHarness(false, workspaceAdmin, hubOwner)
		h.rule.off = true
		_, err := h.set(hubID, true)
		if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != "containers.private_hubs_off" {
			t.Fatalf("error %v, want containers.private_hubs_off", err)
		}
		if h.containers.stored[hubID].Private {
			t.Error("the hub turned private")
		}
	})

	t.Run("sharing again", func(t *testing.T) {
		h := newPrivacyHarness(true, hubOwner)
		h.rule.off = true
		if _, err := h.set(hubID, false); err != nil {
			t.Fatalf("an owner could not share their hub again: %v", err)
		}
		if h.containers.stored[hubID].Private {
			t.Error("the hub is still private")
		}
	})
}
