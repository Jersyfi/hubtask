// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// accessStore is the emergency accesses in memory.
type accessStore struct {
	members  []shared.ID
	accesses []domain.EmergencyAccess
}

func (a *accessStore) Insert(_ context.Context, access domain.EmergencyAccess) error {
	a.accesses = append(a.accesses, access)
	return nil
}

func (a *accessStore) Running(_ context.Context, hub, by shared.ID, now time.Time) (domain.EmergencyAccess, bool, error) {
	for _, access := range a.accesses {
		if access.HubID == hub && access.OpenedBy == by && access.RunningAt(now) {
			return access, true, nil
		}
	}
	return domain.EmergencyAccess{}, false, nil
}

func (a *accessStore) List(_ context.Context, hub shared.ID, _ int) ([]domain.EmergencyAccess, error) {
	var out []domain.EmergencyAccess
	for _, access := range slices.Backward(a.accesses) {
		if access.HubID == hub {
			out = append(out, access)
		}
	}
	return out, nil
}

func (a *accessStore) Members(context.Context, shared.ID) ([]shared.ID, error) { return a.members, nil }

func (a *accessStore) NoticesFor(context.Context, shared.ID, time.Time) ([]domain.EmergencyAccess, error) {
	return a.accesses, nil
}

func (a *accessStore) Find(_ context.Context, id shared.ID) (domain.EmergencyAccess, string, error) {
	for _, access := range a.accesses {
		if access.ID == id {
			return access, "Presents", nil
		}
	}
	return domain.EmergencyAccess{}, "", shared.ErrNotFound
}

// hubRows is the hub's row, private or not, or nothing.
type hubRows struct {
	hub    identityrepository.Hub
	found  bool
	locked int
}

func (h *hubRows) LockHubOf(context.Context, shared.ID) (identityrepository.Hub, bool, error) {
	h.locked++
	return h.hub, h.found, nil
}

func (h *hubRows) GroupHoldsRoleUnder(context.Context, shared.ID) (bool, error) { return false, nil }

// noticesTold records who was told, or fails as a mail outage inside the transaction would not.
type noticesTold struct {
	told [][]shared.ID
	err  error
}

func (n *noticesTold) EmergencyAccessOpened(_ context.Context, _ domain.EmergencyAccess, members []shared.ID) error {
	if n.err != nil {
		return n.err
	}
	n.told = append(n.told, members)
	return nil
}

// movingClock is a clock a test moves.
type movingClock struct{ at time.Time }

func (c *movingClock) Now() time.Time   { return c.at }
func (c *movingClock) Set(at time.Time) { c.at = at }

type proofs struct{ consumed int }

func (p *proofs) Available() bool { return true }
func (p *proofs) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}
func (p *proofs) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	if token != "hbt_sup_fresh" {
		return false, nil
	}
	p.consumed++
	return true, nil
}

type openingHarness struct {
	registry   *usecase.Registry
	accesses   *accessStore
	hubs       *hubRows
	notices    *noticesTold
	proofs     *proofs
	audit      *sink
	authorizer *authorizer
	clock      *movingClock
}

func newOpeningHarness() *openingHarness {
	h := &openingHarness{
		accesses: &accessStore{members: []shared.ID{memberID}},
		hubs:     &hubRows{hub: identityrepository.Hub{ID: hubID, Private: true}, found: true},
		notices:  &noticesTold{}, proofs: &proofs{}, audit: &sink{}, authorizer: &authorizer{},
		clock: &movingClock{at: now},
	}
	registry, err := usecase.NewRegistry(nil, OpenPrivateHub{
		Accesses: h.accesses, Hubs: h.hubs, Notices: h.notices, Authorizer: h.authorizer,
		StepUp: h.proofs, Audit: h.audit, UnitOfWork: &unitOfWork{}, Clock: h.clock, IDs: &ids{},
	}.Descriptor())
	if err != nil {
		panic(err)
	}
	h.registry = registry
	return h
}

var memberID = shared.ID("0192f000-0000-7000-8000-0000000000c7")

func (h *openingHarness) open(actor appshared.ActorContext, reason, proof string) (usecase.Output, error) {
	input := usecase.Input{"hub_id": hubID.String(), "reason": reason}
	if proof != "" {
		input["step_up_token"] = proof
	}
	return h.registry.Invoke(context.Background(), OpenPrivateHubName, actor, input)
}

// UC-ID-16 check 4: the owner, with a fresh proof and a reason, opens the hub for one hour; every
// member is told in the same step, and the trail records it with the reason masked.
func TestTheOwnerOpensAPrivateHubAndEveryMemberIsTold(t *testing.T) {
	h := newOpeningHarness()

	out, err := h.open(actor(), "Maria is in hospital", "hbt_sup_fresh")
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if out["hub_id"] != hubID.String() || out["reason"] != "Maria is in hospital" ||
		out["ends_at"] != now.Add(time.Hour) {
		t.Errorf("answered %v", out)
	}
	asked := h.authorizer.requests[0]
	if asked.Permission != service.PermissionDeleteContainer ||
		!slices.Equal(asked.Path, []identity.Scope{identity.TenantScope()}) {
		t.Errorf("asked %v along %v, want the owner's permission on the workspace alone", asked.Permission, asked.Path)
	}
	if len(h.notices.told) != 1 || !slices.Equal(h.notices.told[0], []shared.ID{memberID}) {
		t.Errorf("told %v, want the member", h.notices.told)
	}
	if h.proofs.consumed != 1 || len(h.accesses.accesses) != 1 {
		t.Errorf("proofs %d, accesses %d", h.proofs.consumed, len(h.accesses.accesses))
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d trail entries, want one", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != EmergencyAccessOpenedAction || entry.TargetID != hubID ||
		entry.Severity != audit.SeverityNotice {
		t.Errorf("the entry is %+v", entry)
	}
	if strings.Contains(stringOf(entry.Changes), "hospital") {
		t.Errorf("the reason is in the trail in clear: %v", entry.Changes)
	}
}

// Every refusal of § 2(b): none of them writes an access, tells anybody or spends the proof where
// it could be avoided.
func TestWhatTheEmergencyAccessRefuses(t *testing.T) {
	service := appshared.ActorContext{
		Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: accountID,
		Scopes: []string{"containers:write"},
	}
	for _, tc := range []struct {
		name    string
		arrange func(*openingHarness)
		actor   appshared.ActorContext
		reason  string
		proof   string
		is      error
		code    string
		spent   bool
	}{
		{name: "not the owner", arrange: func(h *openingHarness) {
			h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")
		}, is: shared.ErrForbidden, code: "access.not_permitted"},
		{name: "a service account", actor: service, is: shared.ErrForbidden, code: "access.not_permitted"},
		{name: "no reason", reason: " ", is: shared.ErrValidation, code: "containers.emergency_reason_required"},
		{name: "a reason too long", reason: strings.Repeat("x", 501), is: shared.ErrValidation, code: "containers.emergency_reason_too_long"},
		{name: "no hub there", arrange: func(h *openingHarness) { h.hubs.found = false },
			is: shared.ErrNotFound, code: "containers.not_found"},
		{name: "a collection", arrange: func(h *openingHarness) { h.hubs.hub.ID = "0192f000-0000-7000-8000-0000000000ff" },
			is: shared.ErrNotFound, code: "containers.not_found"},
		{name: "a shared hub", arrange: func(h *openingHarness) { h.hubs.hub.Private = false },
			is: shared.ErrConflict, code: "containers.emergency_not_private"},
		{name: "the owner is a member", arrange: func(h *openingHarness) { h.accesses.members = []shared.ID{accountID} },
			is: shared.ErrConflict, code: "containers.emergency_not_needed"},
		{name: "no proof", proof: "-", is: shared.ErrForbidden, code: "auth.step_up_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newOpeningHarness()
			if tc.arrange != nil {
				tc.arrange(h)
			}
			who := actor()
			if tc.actor.Kind != "" {
				who = tc.actor
			}
			reason, proof := "Maria is in hospital", "hbt_sup_fresh"
			if tc.reason != "" {
				reason = tc.reason
			}
			if tc.proof == "-" {
				proof = ""
			}
			_, err := h.open(who, reason, proof)
			if !errors.Is(err, tc.is) || shared.AsError(err).DetailCode != tc.code {
				t.Fatalf("error %v, want %s", err, tc.code)
			}
			if len(h.accesses.accesses) != 0 || len(h.notices.told) != 0 || len(h.audit.entries) != 0 {
				t.Error("something was written")
			}
			if h.proofs.consumed != 0 {
				t.Error("the proof was spent on a refusal that could be told first")
			}
		})
	}
}

// Not renewable: a second access while the first runs is refused, naming its end; after the hour a
// new one is a new act, told again.
func TestTheHourIsNotRenewed(t *testing.T) {
	h := newOpeningHarness()
	if _, err := h.open(actor(), "the insurance papers", "hbt_sup_fresh"); err != nil {
		t.Fatal(err)
	}
	h.clock.Set(now.Add(59 * time.Minute))
	_, err := h.open(actor(), "the insurance papers", "hbt_sup_fresh")
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "containers.emergency_running" ||
		shared.AsError(err).Params["ends_at"] != now.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("a second access while one runs: %v", err)
	}
	h.clock.Set(now.Add(time.Hour))
	if _, err := h.open(actor(), "the bank's letter", "hbt_sup_fresh"); err != nil {
		t.Fatalf("a new access after the hour: %v", err)
	}
	if len(h.notices.told) != 2 {
		t.Errorf("the members were told %d times, want once per access", len(h.notices.told))
	}
}

// The members' records and the access commit together or not at all: a failure telling them opens
// nothing.
func TestNothingOpensWhenTheMembersCannotBeTold(t *testing.T) {
	h := newOpeningHarness()
	h.notices.err = shared.ErrUnavailable.WithDetail("postgres.query_failed")
	if _, err := h.open(actor(), "Maria is in hospital", "hbt_sup_fresh"); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want the failure", err)
	}
	if len(h.audit.entries) != 0 {
		t.Error("an opening nobody was told of reached the trail")
	}
}

func stringOf(changes map[string]any) string {
	var b strings.Builder
	for key, value := range changes {
		b.WriteString(key)
		b.WriteString("=")
		if text, ok := value.(string); ok {
			b.WriteString(text)
		} else if nested, ok := value.(map[string]any); ok {
			b.WriteString(stringOf(nested))
		}
		b.WriteString(";")
	}
	return b.String()
}
