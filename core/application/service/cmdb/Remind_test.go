// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type reminders struct {
	writer, campaign shared.ID
	owners           []shared.ID
	calls            int
}

func (r *reminders) AttestationReminder(_ context.Context, _, writer, campaign shared.ID, owners []shared.ID) error {
	r.writer, r.campaign, r.owners, r.calls = writer, campaign, owners, r.calls+1
	return nil
}

// UC-AST-10 check 7: a writer reminds the owners of objects a round still waits for, each once per call;
// the answer and the trail say how many, never whom (rule 10). With every owned object answered
// nobody is reminded and it is refused; a closed round is refused.
func TestARoundRemindsTheOwnersItWaitsFor(t *testing.T) {
	f, a := assuredFixture()
	told := &reminders{}
	f.inv.Notify, f.inv.Reader = told, seeing{}
	started, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Q4", "classes": []any{"server", "virtual_machine"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := shared.ID(started["id"].(string))
	remind := RemindCiCampaign{Inventory: f.inv}.Descriptor()
	out, err := call(t, remind, as(anna), usecase.Input{"campaign_id": id.String()})
	if err != nil {
		t.Fatal(err)
	}
	// host1 is Bert's; vm1 has no owner and reminds nobody.
	if out["reminded"] != 1 || told.calls != 1 || len(told.owners) != 1 || told.owners[0] != bert || told.writer != anna || told.campaign != id {
		t.Errorf("reminded %v: %+v", out["reminded"], told)
	}
	entries := f.inv.Audit.(*trail).entries
	last := entries[len(entries)-1]
	if last.Action != CampaignRemindedAction || last.TargetID != id || len(last.Changes) != 1 || last.Changes["reminded"] == nil {
		t.Errorf("trail: %s %s %v", last.Action, last.TargetID, last.Changes)
	}

	if _, err := call(t, AttestCi{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"campaign_id": id.String(), "ci_id": host.String(), "verdict": "CONFIRMED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, remind, as(anna), usecase.Input{"campaign_id": id.String()}); !errors.Is(err, shared.ErrConflict) || told.calls != 1 {
		t.Errorf("nobody waits, yet: %v (%d calls)", err, told.calls)
	}

	a.asked[id] = append(a.asked[id], domain.Attestation{CampaignID: id, CIID: booking, Owner: bert, Verdict: domain.VerdictPending})
	if _, err := call(t, CloseCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{"campaign_id": id.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, remind, as(anna), usecase.Input{"campaign_id": id.String()}); !errors.Is(err, shared.ErrConflict) || told.calls != 1 {
		t.Errorf("a closed round reminded: %v", err)
	}
}

// gone reads every hub except for the people who lost it.
type gone struct{ lost shared.ID }

func (g gone) Permitted(_ context.Context, actor appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i := range paths {
		out[i] = actor.AccountID != g.lost
	}
	return out, nil
}

// UC-AST-10 check 7: an owner who no longer reads the hub when the reminder is sent is not
// reminded, though the round still names them (rule 2); when only such owners wait, nobody is
// told and it is refused. The trail counts the people told (rule 10).
func TestARoundRemindsOnlyTheOwnersWhoStillReadTheHub(t *testing.T) {
	f, a := assuredFixture()
	told := &reminders{}
	carla := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d0c1")
	f.inv.Notify, f.inv.Reader = told, gone{lost: carla}
	started, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Q4", "classes": []any{"server", "virtual_machine"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := shared.ID(started["id"].(string))
	a.asked[id] = append(a.asked[id], domain.Attestation{CampaignID: id, CIID: booking, Owner: carla, Verdict: domain.VerdictPending})
	remind := RemindCiCampaign{Inventory: f.inv}.Descriptor()
	out, err := call(t, remind, as(anna), usecase.Input{"campaign_id": id.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out["reminded"] != 1 || told.calls != 1 || len(told.owners) != 1 || told.owners[0] != bert {
		t.Errorf("reminded %v: %+v", out["reminded"], told)
	}
	entries := f.inv.Audit.(*trail).entries
	if last := entries[len(entries)-1]; last.Action != CampaignRemindedAction || last.Changes["reminded"] == nil {
		t.Errorf("trail: %s %v", last.Action, last.Changes)
	}

	if _, err := call(t, AttestCi{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"campaign_id": id.String(), "ci_id": host.String(), "verdict": "CONFIRMED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, remind, as(anna), usecase.Input{"campaign_id": id.String()}); !errors.Is(err, shared.ErrConflict) || told.calls != 1 {
		t.Errorf("only a person without the hub waits, yet: %v (%d calls)", err, told.calls)
	}
}
