// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type movePermits struct{ refuse map[shared.ID]bool }

func (p movePermits) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	for _, scope := range request.Path {
		if p.refuse[scope.ID] {
			return false, nil
		}
	}
	return true, nil
}

type moveLeftovers struct{ releases, clocks int }

func (l moveLeftovers) KeptReleaseEntries(context.Context, shared.ID, shared.ID) (int, error) {
	return l.releases, nil
}
func (l moveLeftovers) KeptSlaClocks(context.Context, shared.ID, shared.ID) (int, error) {
	return l.clocks, nil
}

func codes(findings []MoveFinding) map[string]MoveFinding {
	out := map[string]MoveFinding{}
	for _, f := range findings {
		out[f.Code] = f
	}
	return out
}

// UC-WRK-39 checks 1 and 3: a move with nothing in the way is movable; its consequences name who
// loses access, the destination's structure, and what stays at the hub it leaves; nothing is
// announced or recorded.
func TestAPreviewNamesTheConsequencesAndAnnouncesNothing(t *testing.T) {
	h := moving(t)
	preview, err := PreviewContainerMove{Move: h.move(), Permits: movePermits{}, Leftovers: moveLeftovers{releases: 3, clocks: 2}}.
		Execute(t.Context(), actor(), MoveContainerCommand{ContainerID: shoppingID, TargetParentID: otherHubID})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(preview.Obstacles) != 0 {
		t.Errorf("obstacles %+v, want none", preview.Obstacles)
	}
	found := codes(preview.Consequences)
	if found["containers.move_loses_access"].Params["account_ids"] != accountID.String() {
		t.Errorf("consequences %+v, want the account that loses access", preview.Consequences)
	}
	if found["containers.move_takes_structure"].Params["hub_id"] != otherHubID.String() {
		t.Errorf("consequences %+v, want the destination's structure", preview.Consequences)
	}
	if found["containers.move_keeps_releases"].Params["count"] != "3" || found["containers.move_keeps_sla_policies"].Params["count"] != "2" {
		t.Errorf("consequences %+v, want the releases and clocks left behind", preview.Consequences)
	}
	if len(h.events.appended) != 0 || len(h.audit.entries) != 0 || len(h.revoker.announced) != 0 {
		t.Errorf("a preview announced %d events, %d audit entries, %d losses", len(h.events.appended), len(h.audit.entries), len(h.revoker.announced))
	}
}

// UC-WRK-39 check 2: a name taken and a destination without STRUCTURE are obstacles with the
// codes the move refuses with; the move is then not movable.
func TestAPreviewAnswersTheMovesRefusalsAsObstacles(t *testing.T) {
	h := moving(t)
	h.containers.writeErr = shared.ErrConflict.WithDetail("containers.name_taken").WithParams(map[string]string{"name": "Shopping"})
	registry, err := usecase.NewRegistry(nil, PreviewContainerMove{
		Move: h.move(), Permits: movePermits{refuse: map[shared.ID]bool{otherHubID: true}},
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	out, err := registry.Invoke(t.Context(), PreviewContainerMoveName, actor(), usecase.Input{
		"container_id": shoppingID.String(), "target_parent_id": otherHubID.String(),
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	obstacles, _ := out["obstacles"].([]usecase.Output)
	seen := map[string]bool{}
	for _, o := range obstacles {
		seen[o.String("code")] = true
	}
	if out["movable"] != false || !seen["containers.name_taken"] || !seen["access.not_permitted"] {
		t.Errorf("answer %v, want not movable with containers.name_taken and access.not_permitted", out)
	}
}

// UC-WRK-39 check 4: a collection the workspace does not hold answers as a missing one.
func TestAPreviewOfAnUnknownCollectionIsNotFound(t *testing.T) {
	h := moving(t)
	_, err := PreviewContainerMove{Move: h.move()}.Execute(t.Context(), actor(), MoveContainerCommand{
		ContainerID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ff"), TargetParentID: otherHubID,
	})
	if shared.AsError(err).Code != "not_found" {
		t.Errorf("an unknown collection: %v, want not_found", err)
	}
}
