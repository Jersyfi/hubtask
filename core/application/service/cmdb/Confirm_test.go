// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

func (a *assured) Campaigns(_ context.Context, hub shared.ID, _ int) ([]domain.Campaign, error) {
	var out []domain.Campaign
	for _, c := range a.campaigns {
		if c.HubID == hub {
			out = append(out, c)
		}
	}
	return out, nil
}

// asked is a gate that remembers each permission asked and refuses a write to Bert.
type asked struct{ permissions []service.Permission }

func (g *asked) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	g.permissions = append(g.permissions, r.Permission)
	if r.Permission == service.PermissionWriteItems && actor.AccountID == bert {
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	return nil
}

// UC-AST-10: the owning person confirms their object with READ and its review moves on from the
// workspace's day; the object keeps who confirmed it, a pending question of an open campaign is
// answered CONFIRMED and the round counts it; anybody else needs WRITE_ITEMS; the trail names the
// object and the new day only (rule 10).
func TestAnOwnerConfirmsAnObjectAndAnOpenRoundCountsIt(t *testing.T) {
	f, a := assuredFixture()
	gate := &asked{}
	f.inv.Authorizer = gate
	started, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Q4", "classes": []any{"server", "virtual_machine"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	campaign := shared.ID(started["id"].(string))
	confirm := ConfirmCi{Inventory: f.inv}.Descriptor()

	gate.permissions = nil
	out, err := call(t, confirm, as(bert), usecase.Input{"ci_id": host.String(), "review_months": 6})
	if err != nil {
		t.Fatalf("the owner could not confirm: %v", err)
	}
	if len(gate.permissions) != 1 || gate.permissions[0] != service.PermissionRead {
		t.Errorf("the owner was asked for %v, want READ", gate.permissions)
	}
	ci := f.m.cis[host]
	if ci.Compliance.ReviewDue == nil || ci.Compliance.ReviewDue.Format(time.DateOnly) != "2027-04-29" {
		t.Errorf("next review %v, want 2027-04-29", ci.Compliance.ReviewDue)
	}
	if ci.ConfirmedBy != bert || ci.ConfirmedAt == nil || !ci.ConfirmedAt.Equal(start) {
		t.Errorf("confirmed by %s at %v", ci.ConfirmedBy, ci.ConfirmedAt)
	}
	if out["confirmed_by"] != bert.String() || out["confirmed_at"] != start {
		t.Errorf("answer: %v / %v", out["confirmed_by"], out["confirmed_at"])
	}
	at, _ := a.FindAttestation(context.Background(), campaign, host)
	if at.Verdict != domain.VerdictConfirmed || at.AttestedBy != bert {
		t.Errorf("the round's question: %+v", at)
	}
	read, _ := call(t, ReadCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{"campaign_id": campaign.String()})
	if score := read["score"].(usecase.Output); score["confirmed"] != 1 || score["pending"] != 1 {
		t.Errorf("score %v", score)
	}
	entries := f.inv.Audit.(*trail).entries
	last := entries[len(entries)-1]
	if last.Action != CIConfirmedAction || last.TargetID != host {
		t.Errorf("trail: %s on %s", last.Action, last.TargetID)
	}
	if len(last.Changes) != 2 || last.Changes["review_due"] == nil || last.Changes["campaigns_answered"] == nil {
		t.Errorf("trail changes %v, want the day and the rounds answered only", last.Changes)
	}

	if _, err := call(t, confirm, as(bert), usecase.Input{"ci_id": vm1.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("Bert confirmed an object he does not own: %v", err)
	}
	if _, err := call(t, confirm, as(anna), usecase.Input{"ci_id": vm1.String()}); err != nil {
		t.Fatalf("a writer could not confirm: %v", err)
	}
	if due := f.m.cis[vm1].Compliance.ReviewDue; due == nil || due.Format(time.DateOnly) != "2027-10-29" {
		t.Errorf("vm1 without a period: %v, want a year on", due)
	}
	if _, err := call(t, confirm, as(anna), usecase.Input{"ci_id": vm1.String(), "review_months": 61}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("61 months: %v", err)
	}
}

// A CONFIRMED answer in a round records who confirmed the object, as its own act does; a
// DISPUTED one does not.
func TestARoundsConfirmationIsTheObjectsConfirmation(t *testing.T) {
	f, _ := assuredFixture()
	started, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Q4", "classes": []any{"server", "virtual_machine"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	attest := AttestCi{Inventory: f.inv}.Descriptor()
	for ci, verdict := range map[shared.ID]string{host: "CONFIRMED", vm1: "DISPUTED"} {
		if _, err := call(t, attest, as(anna), usecase.Input{"campaign_id": started["id"], "ci_id": ci.String(), "verdict": verdict}); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.m.cis[host]; got.ConfirmedBy != anna || got.ConfirmedAt == nil {
		t.Errorf("confirmed in the round: %s %v", got.ConfirmedBy, got.ConfirmedAt)
	}
	if got := f.m.cis[vm1]; !got.ConfirmedBy.IsZero() || got.ConfirmedAt != nil {
		t.Errorf("disputed yet confirmed: %s %v", got.ConfirmedBy, got.ConfirmedAt)
	}
}
