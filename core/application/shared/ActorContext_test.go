// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	tenant  = domain.ID("018f0000-0000-7000-8000-000000000001")
	account = domain.ID("018f0000-0000-7000-8000-000000000002")
)

func TestAnonymousIsNotAuthenticated(t *testing.T) {
	actor := Anonymous("de-AT", "Europe/Vienna")

	if actor.IsAuthenticated() {
		t.Error("the anonymous actor is authenticated")
	}
	if actor.Locale != "de-AT" || actor.TimeZone != "Europe/Vienna" {
		t.Errorf("locale %q, time zone %q", actor.Locale, actor.TimeZone)
	}
	if !actor.TenantID.IsZero() {
		t.Errorf("tenant %q on an anonymous actor", actor.TenantID)
	}
}

// A half-built context must not read as authenticated - that is the failure mode this guards.
func TestAHalfBuiltActorIsNotAuthenticated(t *testing.T) {
	cases := map[string]ActorContext{
		"the zero value":          {},
		"a kind without a tenant": {Kind: ActorUser},
		"a tenant without a kind": {TenantID: tenant},
		"anonymous with a tenant": {Kind: ActorAnonymous, TenantID: tenant},
	}

	for name, actor := range cases {
		t.Run(name, func(t *testing.T) {
			if actor.IsAuthenticated() {
				t.Error("counted as authenticated")
			}
		})
	}
}

func TestAnAuthenticatedActorCarriesItsTenant(t *testing.T) {
	actor := ActorContext{Kind: ActorUser, TenantID: tenant, AccountID: account}

	if !actor.IsAuthenticated() {
		t.Fatal("not authenticated")
	}
	scope := actor.PersistenceScope()
	if scope.TenantID != tenant || scope.ActorID != account {
		t.Errorf("persistence scope = %+v", scope)
	}
	if !scope.IsValid() {
		t.Error("the unit of work would refuse this scope")
	}
}

func TestRequireScope(t *testing.T) {
	authenticated := ActorContext{
		Kind: ActorServiceAccount, TenantID: tenant, Scopes: []string{"items:read", "items:write"},
	}

	t.Run("a scope the token carries", func(t *testing.T) {
		if err := authenticated.RequireScope("items:write"); err != nil {
			t.Errorf("refused: %v", err)
		}
	})

	t.Run("a scope the token lacks names it", func(t *testing.T) {
		err := authenticated.RequireScope("automation:manage")
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error = %v, want forbidden", err)
		}
		var domainErr *domain.Error
		if !errors.As(err, &domainErr) {
			t.Fatal("not a domain error")
		}
		if domainErr.Params["scope"] != "automation:manage" {
			t.Errorf("params = %v - the client cannot tell which scope to ask for", domainErr.Params)
		}
	})

	t.Run("without a credential it is 401, not 403", func(t *testing.T) {
		// The difference matters to a client: one says "sign in", the other says "you never
		// will".
		if err := Anonymous("en", "UTC").RequireScope("items:read"); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("error = %v, want unauthenticated", err)
		}
	})
}

func TestHasScope(t *testing.T) {
	actor := ActorContext{Scopes: []string{"items:read"}}

	if !actor.HasScope("items:read") {
		t.Error("a carried scope was not found")
	}
	// A prefix is not a scope: `items:read` must not satisfy `items:readwrite` or the reverse.
	if actor.HasScope("items:readwrite") || actor.HasScope("items") {
		t.Error("a scope matched by prefix")
	}
}

func TestTheActorTravelsInTheContext(t *testing.T) {
	actor := ActorContext{Kind: ActorUser, TenantID: tenant, Locale: "en"}

	found, ok := ActorFrom(ContextWithActor(context.Background(), actor))
	if !ok {
		t.Fatal("the actor did not come back out")
	}
	if found.TenantID != tenant || found.Kind != ActorUser {
		t.Errorf("actor = %+v", found)
	}
}

// A context without the middleware must not yield a usable actor.
func TestAContextWithoutAnActorSaysSo(t *testing.T) {
	actor, ok := ActorFrom(context.Background())
	if ok {
		t.Fatal("an actor appeared out of nowhere")
	}
	if actor.IsAuthenticated() {
		t.Error("the fallback actor is authenticated")
	}
	if actor.PersistenceScope().IsValid() {
		t.Error("the fallback actor could open a transaction")
	}
}

// The two facts a push adds to every writer without touching one (N-04, N-05): the device the
// context is marked with, and the readings its fields were written under. Absent, both answer
// nothing - which is what a write through the API is.
func TestAContextCarriesTheDeviceAndTheReadingsAPushMarkedItWith(t *testing.T) {
	ctx := context.Background()
	if got := DeviceFrom(ctx); !got.IsZero() {
		t.Errorf("an unmarked context names device %q", got)
	}
	if _, found := ReadingFrom(ctx, "title"); found {
		t.Errorf("an unmarked context carries a reading")
	}

	device := domain.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	reading, err := domain.NewHLC(time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), 1, "dev-a")
	if err != nil {
		t.Fatalf("building the reading: %v", err)
	}
	marked := ContextWithReadings(ContextWithDevice(ctx, device), map[string]domain.HLC{"title": reading})

	if got := DeviceFrom(marked); got != device {
		t.Errorf("the device is %q, want %s", got, device)
	}
	if got, found := ReadingFrom(marked, "title"); !found || got.Compare(reading) != 0 {
		t.Errorf("the title's reading is %s (found=%v), want the device's", got, found)
	}
	// A field the push did not name keeps the writer's own reading.
	if _, found := ReadingFrom(marked, "notes"); found {
		t.Errorf("a field the push did not name carries a reading")
	}
	// And a zero reading is no reading: the writer's stays.
	if _, found := ReadingFrom(ContextWithReadings(ctx, map[string]domain.HLC{"title": {}}), "title"); found {
		t.Errorf("a zero reading counted as one")
	}
}
