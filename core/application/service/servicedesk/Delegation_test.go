// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// delegating keeps one delegation per person, as the table does.
type delegating struct {
	*approvalStore
	held map[shared.ID]domain.Delegation
}

func (d *delegating) Delegate(_ context.Context, delegation domain.Delegation) error {
	d.held[delegation.AccountID] = delegation
	return nil
}

func (d *delegating) DelegationOf(_ context.Context, account shared.ID) (domain.Delegation, error) {
	found, ok := d.held[account]
	if !ok {
		return domain.Delegation{}, shared.ErrNotFound
	}
	return found, nil
}

func (d *delegating) EndDelegation(_ context.Context, account shared.ID) (bool, error) {
	_, ok := d.held[account]
	delete(d.held, account)
	return ok, nil
}

// UC-SVC-10 check 1: a person hands their approvals to somebody for a while - never to
// themselves, never for an empty span - a new delegation replaces the earlier one, it is read
// back and ended, and setting and ending it are on the trail.
func TestAPersonHandsTheirApprovalsToSomebodyWhileAway(t *testing.T) {
	approvals, store, _, _ := approvalFixture(t)
	held := &delegating{approvalStore: store, held: map[shared.ID]domain.Delegation{}}
	trail := &trailOf{}
	approvals.Store, approvals.Desk.Audit = held, trail
	doors, err := usecase.NewRegistry(nil,
		SetApprovalDelegation{Approvals: approvals}.Descriptor(),
		ReadApprovalDelegation{Approvals: approvals}.Descriptor(),
		EndApprovalDelegation{Approvals: approvals}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	ctx, away := context.Background(), person(handlerID)
	deputy := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f9")
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fa")
	from, until := monday.Format(time.RFC3339), monday.AddDate(0, 0, 14).Format(time.RFC3339)

	for _, c := range []struct {
		name string
		in   usecase.Input
		code string
	}{
		{"to themselves", usecase.Input{"delegate_id": handlerID.String(), "from": from, "until": until}, "approvals.delegate_self"},
		{"an end before the start", usecase.Input{"delegate_id": deputy.String(), "from": until, "until": from}, "approvals.delegation_empty"},
		{"longer than a year", usecase.Input{"delegate_id": deputy.String(), "from": from,
			"until": monday.AddDate(2, 0, 0).Format(time.RFC3339)}, "approvals.delegation_too_long"},
	} {
		if _, err := doors.Invoke(ctx, SetApprovalDelegationName, away, c.in); shared.AsError(err) == nil || shared.AsError(err).DetailCode != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if len(held.held) != 0 || len(trail.entries) != 0 {
		t.Fatal("a refused delegation was written")
	}

	read, err := doors.Invoke(ctx, ReadApprovalDelegationName, away, usecase.Input{})
	// A nil output, which the doors write as null.
	if err != nil || read["delegation"].(usecase.Output) != nil {
		t.Fatalf("no delegation reads as null: %v %v", read, err)
	}
	if _, err := doors.Invoke(ctx, SetApprovalDelegationName, away, usecase.Input{"delegate_id": deputy.String(), "from": from, "until": until}); err != nil {
		t.Fatalf("delegating: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetApprovalDelegationName, away, usecase.Input{"delegate_id": other.String(), "from": from, "until": until}); err != nil {
		t.Fatalf("delegating anew: %v", err)
	}
	read, err = doors.Invoke(ctx, ReadApprovalDelegationName, away, usecase.Input{})
	if err != nil || read["delegation"].(usecase.Output)["delegate_id"] != other.String() {
		t.Fatalf("the new delegation replaces the earlier one: %v %v", read, err)
	}
	if _, err := doors.Invoke(ctx, EndApprovalDelegationName, away, usecase.Input{}); err != nil {
		t.Fatalf("ending: %v", err)
	}
	if _, err := doors.Invoke(ctx, EndApprovalDelegationName, away, usecase.Input{}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("ending what is not there: %v", err)
	}
	actions := []string{}
	for _, e := range trail.entries {
		actions = append(actions, string(e.Action))
	}
	if len(actions) != 3 || actions[2] != string(DelegationEndedAction) {
		t.Fatalf("the trail: %v", actions)
	}
}

// A moment that is no RFC 3339 moment is refused by name rather than read as the zero time.
func TestADelegationsMomentMustBeAMoment(t *testing.T) {
	approvals, _, _, _ := approvalFixture(t)
	_, err := SetApprovalDelegation{Approvals: approvals}.Descriptor().Handler.Invoke(context.Background(), person(handlerID),
		usecase.Input{"delegate_id": handlerID.String(), "from": "next monday", "until": "2026-11-20T00:00:00Z"})
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != "approvals.moment_malformed" {
		t.Fatalf("a malformed moment: %v", err)
	}
}
