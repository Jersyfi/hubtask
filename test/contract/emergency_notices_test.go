// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// toldAccesses is one access told to one member, held in memory: the store the three use cases
// read, so that what the router answers is what they decide.
type toldAccesses struct {
	access       workdomain.EmergencyAccess
	member       shared.ID
	acknowledged map[shared.ID]time.Time
}

func (s *toldAccesses) Insert(context.Context, workdomain.EmergencyAccess) error { return nil }
func (s *toldAccesses) Running(context.Context, shared.ID, shared.ID, time.Time) (workdomain.EmergencyAccess, bool, error) {
	return workdomain.EmergencyAccess{}, false, nil
}
func (s *toldAccesses) List(context.Context, shared.ID, int) ([]workdomain.EmergencyAccess, error) {
	return []workdomain.EmergencyAccess{s.access}, nil
}
func (s *toldAccesses) Members(context.Context, shared.ID) ([]shared.ID, error) {
	return []shared.ID{s.member}, nil
}
func (s *toldAccesses) NoticesFor(_ context.Context, recipient shared.ID, _ time.Time) ([]workdomain.EmergencyNotice, error) {
	if recipient != s.member {
		return nil, nil
	}
	return []workdomain.EmergencyNotice{{Access: s.access, AcknowledgedAt: s.acknowledged[recipient]}}, nil
}
func (s *toldAccesses) Acknowledge(_ context.Context, _, account shared.ID, at time.Time) (bool, error) {
	if _, held := s.acknowledged[account]; held {
		return false, nil
	}
	s.acknowledged[account] = at
	return true, nil
}
func (s *toldAccesses) Find(context.Context, shared.ID) (workdomain.EmergencyAccess, string, error) {
	return s.access, "", nil
}

type directly struct{}

func (directly) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (directly) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type permitted struct{}

func (permitted) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type kept struct{ entries []audit.Entry }

func (k *kept) Append(_ context.Context, entry audit.Entry) error {
	k.entries = append(k.entries, entry)
	return nil
}

// UC-ID-16 check 10 at the REST door: confirming answers 204, twice 204, an access the caller was
// not told of 404; the member's notices answer `acknowledged_at` as the contract's EmergencyNotice,
// null and then the moment; the hub's list of accesses stays PrivateHubAccess and says nothing of
// who confirmed.
func TestAConfirmedNoticeAtTheRestDoor(t *testing.T) {
	spec := contractSpec(t)
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	member := shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	opened := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	store := &toldAccesses{
		access: workdomain.EmergencyAccess{
			ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), TenantID: tenant,
			HubID:    shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
			OpenedBy: shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"),
			Reason:   "Maria is in hospital", OpenedAt: opened, EndsAt: opened.Add(time.Hour),
		},
		member: member, acknowledged: map[shared.ID]time.Time{},
	}
	at := portclock.Fixed(opened.Add(2 * time.Hour))
	registry, err := catalogue.NewRegistry(nil,
		work.AcknowledgeEmergencyNotice{Accesses: store, Audit: &kept{}, UnitOfWork: directly{}, Clock: at}.Descriptor(),
		work.ListEmergencyNotices{Accesses: store, UnitOfWork: directly{}, Clock: at}.Descriptor(),
		work.ListPrivateHubAccesses{Accesses: store, Authorizer: permitted{}, UnitOfWork: directly{}}.Descriptor(),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := rest.NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: member,
		Scopes: []string{"containers:read", "containers:write"},
	})
	call := func(method, path string) (int, []byte) {
		t.Helper()
		response := httptest.NewRecorder()
		controller.Routes().ServeHTTP(response, httptest.NewRequestWithContext(ctx, method, rest.APIBasePath+path, nil))
		return response.Code, response.Body.Bytes()
	}
	notice := func() map[string]any {
		t.Helper()
		status, raw := call(http.MethodGet, "/accounts/me/emergency-notices")
		if status != http.StatusOK {
			t.Fatalf("the notices: %d %s", status, raw)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil || len(rows) != 1 {
			t.Fatalf("the notices are %s", raw)
		}
		problems, err := spec.validateAgainst("EmergencyNotice", rows[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range problems {
			t.Errorf("EmergencyNotice: %s", problem)
		}
		var row map[string]any
		_ = json.Unmarshal(rows[0], &row)
		return row
	}

	if row := notice(); row["acknowledged_at"] != nil {
		t.Errorf("before confirming: acknowledged_at %v, want null", row["acknowledged_at"])
	} else if _, held := row["acknowledged_at"]; !held {
		t.Error("the notice leaves acknowledged_at out instead of answering null")
	}
	confirm := "/accounts/me/emergency-notices/" + store.access.ID.String() + ":acknowledge"
	for _, attempt := range []string{"first", "second"} {
		if status, raw := call(http.MethodPost, confirm); status != http.StatusNoContent {
			t.Errorf("the %s confirmation: %d %s", attempt, status, raw)
		}
	}
	if row := notice(); row["acknowledged_at"] != "2026-11-20T11:00:00Z" {
		t.Errorf("after confirming: acknowledged_at %v, want the moment", row["acknowledged_at"])
	}
	status, raw := call(http.MethodPost, "/accounts/me/emergency-notices/0192f000-0000-7000-8000-0000000000e9:acknowledge")
	if status != http.StatusNotFound {
		t.Errorf("an access the caller was not told of: %d %s, want 404", status, raw)
	}

	status, raw = call(http.MethodGet, "/private-hubs/"+store.access.HubID.String()+"/accesses")
	if status != http.StatusOK {
		t.Fatalf("the hub's accesses: %d %s", status, raw)
	}
	var accesses []map[string]any
	if err := json.Unmarshal(raw, &accesses); err != nil || len(accesses) != 1 {
		t.Fatalf("the hub's accesses are %s", raw)
	}
	if _, carried := accesses[0]["acknowledged_at"]; carried {
		t.Errorf("the hub's list of accesses says who confirmed: %s", raw)
	}
	schema, _ := spec.resolve(spec.Components.Schemas["PrivateHubAccess"])
	if _, carried := schema.Properties["acknowledged_at"]; carried {
		t.Error("PrivateHubAccess, which the owner's list answers, has an acknowledged_at")
	}
	if !slices.Contains(spec.Components.Schemas["EmergencyNotice"].Required, "acknowledged_at") {
		t.Error("EmergencyNotice does not require acknowledged_at")
	}
}
