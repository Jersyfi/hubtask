// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-JUM-07 check 10: the workspace's own switch for proposals on arrival, off by default, through
// the registry.

func arrivalRegistry(t *testing.T, world *sourceWorld, store *providerStore, authorizer Authorizer, workspaces identityrepo.Workspaces) *usecase.Registry {
	t.Helper()
	writer := AiSourceWriter{
		Rules: AiRules{
			Instance: world, Workspaces: workspaces,
			Offered: sourceOffered{world}, Choices: sourceChoices{world},
			Consents: keepers{},
		},
		Authorizer: authorizer, Audit: store, UnitOfWork: providerDirect{}, Clock: providerClock{},
	}
	registry, err := usecase.NewRegistry(nil, ReadAiSource{Writer: writer}.Descriptor(),
		SetAiArrivalProposals{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry
}

func TestProposalsOnArrivalAreOffUntilTheWorkspaceSwitchesThemOn(t *testing.T) {
	world := &sourceWorld{}
	store := &providerStore{}
	registry := arrivalRegistry(t, world, store, providerAllow{}, sourceWorkspaces{world})

	if out := readSourceVia(t, registry); out["prepare_on_arrival"] != false {
		t.Fatalf("a fresh workspace answered %v", out["prepare_on_arrival"])
	}

	for _, prepare := range []bool{true, false} {
		out, err := registry.Invoke(t.Context(), SetAiArrivalProposalsName, providerActor(),
			usecase.Input{"prepare": prepare})
		if err != nil {
			t.Fatalf("switching to %v: %v", prepare, err)
		}
		if out["prepare_on_arrival"] != prepare || world.workspace.Settings.AiArrivalProposals != prepare {
			t.Errorf("switched to %v, answered %v and stored %v", prepare, out["prepare_on_arrival"],
				world.workspace.Settings.AiArrivalProposals)
		}
		last := store.entries[len(store.entries)-1]
		if last.Action != AiArrivalProposalsSetAction || len(last.Changes) != 1 {
			t.Errorf("the trail holds %+v", last)
		}
	}
}

// refusingAuthorizer refuses everything, as a member, a viewer or an auditor is refused.
type refusingAuthorizer struct{ providerAllow }

func (refusingAuthorizer) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

func TestOnlyWhoMayConfigureAiSwitchesProposalsOnArrival(t *testing.T) {
	world := &sourceWorld{}
	registry := arrivalRegistry(t, world, &providerStore{}, refusingAuthorizer{}, sourceWorkspaces{world})

	_, err := registry.Invoke(t.Context(), SetAiArrivalProposalsName, providerActor(), usecase.Input{"prepare": true})
	if got := shared.AsError(err); got == nil || got.DetailCode != "access.not_permitted" {
		t.Errorf("a refused switch answered %v", err)
	}
	if world.workspace.Settings.AiArrivalProposals {
		t.Error("a refused switch was written")
	}
}

// movingWorkspaces is a workspace somebody else saves between the read and the write.
type movingWorkspaces struct{ sourceWorkspaces }

func (m movingWorkspaces) Find(ctx context.Context) (identity.Workspace, error) {
	read, err := m.sourceWorkspaces.Find(ctx)
	m.world.workspace.Version++
	return read, err
}

func (m movingWorkspaces) Update(ctx context.Context, changed identity.Workspace, expected int, now time.Time) (bool, error) {
	return m.sourceWorkspaces.Update(ctx, changed, expected, now)
}

func TestTwoAdministratorsSwitchingAtOnceAnswerTheSecondAConflict(t *testing.T) {
	world := &sourceWorld{}
	registry := arrivalRegistry(t, world, &providerStore{}, providerAllow{}, movingWorkspaces{sourceWorkspaces{world}})

	_, err := registry.Invoke(t.Context(), SetAiArrivalProposalsName, providerActor(), usecase.Input{"prepare": true})
	if got := shared.AsError(err); got == nil || got.DetailCode != "workspace.version_conflict" {
		t.Errorf("a write over a moved workspace answered %v", err)
	}
	if world.workspace.Settings.AiArrivalProposals {
		t.Error("the write over a moved workspace was stored")
	}
}
