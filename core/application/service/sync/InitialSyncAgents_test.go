// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) AgentModels(_ context.Context, after shared.ID, batch int) ([]agent.Model, error) {
	return pageAfter(s.models, func(m agent.Model) string { return m.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Agents(_ context.Context, after shared.ID, batch int) ([]agent.Agent, error) {
	return pageAfter(s.agents, func(a agent.Agent) string { return a.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) AgentRequests(_ context.Context, after shared.ID, batch int) ([]repository.HeldActOf, error) {
	return pageAfter(s.held, func(h repository.HeldActOf) string { return h.Request.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) AgentCases(_ context.Context, after shared.ID, batch int) ([]agent.Case, error) {
	return pageAfter(s.cases, func(c agent.Case) string { return c.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) AgentRuns(_ context.Context, after shared.ID, batch int) ([]repository.RunOf, error) {
	return pageAfter(s.runs, func(r repository.RunOf) string { return r.Run.ID.String() }, after.String(), batch), nil
}

// UC-AGT-01 … 04 offline: the walk hands an agent, its held acts and its runs to the agent's owner,
// every row of the registry to whoever administers agents, and nothing to anybody else.
func TestTheWalkCarriesTheRegistryToItsReaders(t *testing.T) {
	pull, f, store := walking(t)
	model := idAt('a', 9)
	store.models = []agent.Model{{ID: model, Key: "local", Revision: 2}}
	store.agents = []agent.Agent{
		{ID: idAt('a', 1), Name: "Mine", OwnerID: account, ModelID: model, ApprovedModelRevision: 1},
		{ID: idAt('a', 2), Name: "Theirs", OwnerID: someoneElse},
	}
	store.held = []repository.HeldActOf{
		{Request: agent.Request{ID: idAt('a', 3), AgentID: idAt('a', 1)}, OwnerID: account},
		{Request: agent.Request{ID: idAt('a', 4), AgentID: idAt('a', 2)}, OwnerID: someoneElse},
	}
	store.cases = []agent.Case{{ID: idAt('a', 5), AgentID: idAt('a', 1), Title: "Printer jams"}}
	store.runs = []repository.RunOf{{Run: agent.Run{ID: idAt('a', 6), AgentID: idAt('a', 1)}, OwnerID: account}}

	walked := func() map[string][]shared.ID {
		records, _ := wholeWalk(t, pull, 100)
		out := map[string][]shared.ID{}
		for _, record := range records {
			out[record.Entity] = append(out[record.Entity], record.EntityID)
			if record.Entity == entityAgent && record.EntityID == idAt('a', 1) && record.Payload["needs_approval"] != true {
				t.Errorf("the agent's record = %+v, want it needing approval on its model's new revision", record.Payload)
			}
		}
		return out
	}
	got := walked()
	if !slices.Equal(got[entityAgent], []shared.ID{idAt('a', 1)}) || !slices.Equal(got[entityAgentRequest], []shared.ID{idAt('a', 3)}) ||
		!slices.Equal(got[entityAgentRun], []shared.ID{idAt('a', 6)}) {
		t.Errorf("the owner's walk = %v, want their agent, its held act and its run", got)
	}
	if len(got[entityAgentModel]) != 0 || len(got[entityAgentCase]) != 0 {
		t.Errorf("a model %v or a case %v reached an owner who does not administer agents", got[entityAgentModel], got[entityAgentCase])
	}

	f.auth.members = true
	got = walked()
	if len(got[entityAgent]) != 2 || len(got[entityAgentRequest]) != 2 || len(got[entityAgentModel]) != 1 || len(got[entityAgentCase]) != 1 {
		t.Errorf("the administrator's walk = %v, want every row", got)
	}
}

// The delta delivers an agent's row to its owner and the administrators alone, a case's removal to
// the administrators.
func TestTheDeltaDeliversTheRegistryToItsReaders(t *testing.T) {
	record := func(seq int64, entity string, payload map[string]any) repository.Recorded {
		recorded := entry(seq, "")
		recorded.Entity, recorded.Payload = entity, payload
		return recorded
	}
	removal := record(5, entityAgentCase, nil)
	removal.Op = repository.Delete
	pull, f := pulling(t,
		record(1, entityAgent, map[string]any{"owner_id": account.String()}),
		record(2, entityAgent, map[string]any{"owner_id": someoneElse.String()}),
		record(3, entityAgentRequest, map[string]any{"owner_id": account.String()}),
		record(4, entityAgentModel, map[string]any{"key": "local"}),
		removal,
	)
	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for _, delivered := range page.Records {
		seqs = append(seqs, delivered.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 3}) {
		t.Errorf("the owner's delta = %v, want their agent and its held act", seqs)
	}
	f.auth.members = true
	page, err = pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 5 {
		t.Errorf("the administrator's delta delivered %d records, want all five", len(page.Records))
	}
}
