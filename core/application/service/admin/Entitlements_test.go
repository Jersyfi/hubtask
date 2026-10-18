// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// planWorkspace is the workspace whose plan the gate reads.
const planWorkspace = shared.ID("018f2a1b-0000-7000-8000-0000000000e5")

type leftOutStore struct {
	leftOut entitlement.LeftOut
	err     error
	reads   int
}

func (s *leftOutStore) LeftOut(context.Context) (entitlement.LeftOut, error) {
	s.reads++
	return s.leftOut, s.err
}

// gatedRegistry is a registry with the gate and one operation per feature, plus one that uses
// none, each answering which ran.
func gatedRegistry(t *testing.T, store *leftOutStore, uow *unitOfWork, observed *[]string) *usecase.Registry {
	t.Helper()
	ran := func(name string) usecase.HandlerFunc {
		return func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{"ran": name}, nil
		}
	}
	observe := func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		return usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			*observed = append(*observed, descriptor.Name)
			return next.Invoke(ctx, actor, in)
		})
	}
	registry, err := usecase.NewRegistry(EntitlementGate{Plans: store, UnitOfWork: uow}.Wrap(observe),
		usecase.Descriptor{Name: "CreateThing", Summary: "x", Entitlement: entitlement.AutomationRules, Handler: ran("CreateThing")},
		usecase.Descriptor{Name: "SendThing", Summary: "x", Entitlement: entitlement.Webhooks, Handler: ran("SendThing")},
		usecase.Descriptor{Name: "ReadThing", Summary: "x", Handler: ran("ReadThing")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func member() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: planWorkspace, AccountID: operatorID}
}

// The registry refuses an operation whose feature the caller's plan leaves out, through every
// door, and lets the rest through - read in the caller's own workspace (multi-tenancy.md §4.2).
func TestTheGateRefusesWhatThePlanLeavesOut(t *testing.T) {
	store := &leftOutStore{leftOut: entitlement.LeftOut{entitlement.AutomationRules}}
	uow := &unitOfWork{}
	var observed []string
	registry := gatedRegistry(t, store, uow, &observed)

	_, err := registry.Invoke(context.Background(), "CreateThing", member(), usecase.Input{})
	if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != entitlement.NotIncludedCode ||
		shared.AsError(err).Params["feature"] != string(entitlement.AutomationRules) {
		t.Fatalf("an operation the plan leaves out answered %v", err)
	}
	if len(observed) != 1 {
		t.Errorf("the refusal was not observed: %v", observed)
	}
	if len(uow.scopes) != 1 || uow.scopes[0].TenantID != planWorkspace {
		t.Errorf("read under %v, want the caller's workspace", uow.scopes)
	}

	for _, name := range []string{"SendThing", "ReadThing"} {
		out, err := registry.Invoke(context.Background(), name, member(), usecase.Input{})
		if err != nil || out["ran"] != name {
			t.Errorf("%s answered %v, %v", name, out, err)
		}
	}
	if store.reads != 2 {
		t.Errorf("read %d times: an operation without a feature reads nothing", store.reads)
	}
}

// Included again, the same operation runs: nothing is cached past the request.
func TestTheGateLetsAnIncludedFeatureThrough(t *testing.T) {
	store := &leftOutStore{leftOut: entitlement.LeftOut{entitlement.AutomationRules}}
	var observed []string
	registry := gatedRegistry(t, store, &unitOfWork{}, &observed)
	if _, err := registry.Invoke(context.Background(), "CreateThing", member(), usecase.Input{}); err == nil {
		t.Fatal("left out, it ran")
	}
	store.leftOut = nil
	if out, err := registry.Invoke(context.Background(), "CreateThing", member(), usecase.Input{}); err != nil || out["ran"] != "CreateThing" {
		t.Fatalf("included again, it answered %v, %v", out, err)
	}
}

// An actor without a workspace reads nothing and is refused nothing; an unreadable plan is an
// error, never "included".
func TestTheGateWithoutAWorkspaceAndWithAnUnreadablePlan(t *testing.T) {
	store := &leftOutStore{leftOut: entitlement.Features()}
	var observed []string
	registry := gatedRegistry(t, store, &unitOfWork{}, &observed)
	if _, err := registry.Invoke(context.Background(), "CreateThing", appshared.ActorContext{Kind: appshared.ActorSystem}, usecase.Input{}); err != nil {
		t.Fatalf("an actor without a workspace was refused: %v", err)
	}
	if store.reads != 0 {
		t.Errorf("read the plan of no workspace")
	}

	store.err = shared.ErrUnavailable
	if _, err := registry.Invoke(context.Background(), "SendThing", member(), usecase.Input{}); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("an unreadable plan answered %v", err)
	}
}

func TestARegistrationNamingAnUnknownFeatureIsRefused(t *testing.T) {
	_, err := usecase.NewRegistry(nil, usecase.Descriptor{
		Name: "ExportThing", Summary: "x", Entitlement: "EXPORT",
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return nil, nil
		}),
	})
	if err == nil || shared.AsError(err).DetailCode != "usecase.registration_incomplete" {
		t.Fatalf("a feature outside the set registered: %v", err)
	}
}
