// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	ticket     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000101")
	collection = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000102")
	hubID      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000103")
	printer    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000104")
	router     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000105")
	dupID      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000106")
)

type queued struct{ requests []queue.Request }

func (q *queued) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, r)
	return shared.MustParseID("01936f2a-7c1e-7000-8000-000000000999"), nil
}

// workspace answers the reads a shipped agent makes and records what it attempts, behind the
// gate as the composition root puts it.
type workspace struct {
	gate      Gate
	completed bool
	performed []call
	actors    []appshared.ActorContext
}

func (w *workspace) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	reads := map[string]usecase.Output{
		getWorkItemName: {"id": ticket.String(), "collection_id": collection.String(), "title": "Printer on floor 3 jams",
			"notes": "Since the Router-2 update, the printer stops.", "is_completed": w.completed},
		getContainerName: {"id": collection.String(), "parent_id": hubID.String()},
		listCisName: {"data": []usecase.Output{
			{"id": printer.String(), "name": "Printer"}, {"id": router.String(), "name": "Router-2"},
			{"id": "01936f2a-7c1e-7000-8000-000000000107", "name": "Scanner"},
		}, "next": nil},
		listCiDuplicatesName: {"data": []usecase.Output{{"id": dupID.String(), "ci_ids": []string{printer.String(), router.String()}}}},
		readCiName:           nil,
	}
	_, read := reads[name]
	handler := usecase.HandlerFunc(func(_ context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
		w.actors = append(w.actors, actor)
		if name == readCiName {
			seen := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			if in.String("ci_id") == router.String() {
				return usecase.Output{"id": router.String(), "last_seen_at": seen}, nil
			}
			return usecase.Output{"id": printer.String(), "last_seen_at": nil}, nil
		}
		if read {
			return reads[name], nil
		}
		w.performed = append(w.performed, call{name, actor, in})
		return usecase.Output{}, nil
	})
	return w.gate.Wrap(nil)(usecase.Descriptor{Name: name, ReadOnly: read}, handler).Invoke(ctx, actor, in)
}

type classifier struct{ tokens int64 }

func (c classifier) Classify(context.Context, appshared.ActorContext, shared.ID) (Proposal, error) {
	return Proposal{Recorded: true, Code: "suggestions.proposed", Tokens: c.tokens}, nil
}

func (f fixture) shipped(t *testing.T, kind string, in usecase.Input) (domain.Agent, *queued) {
	t.Helper()
	q := &queued{}
	f.desk.Queue = q
	base := usecase.Input{"kind": kind, "policy": map[string]any{"reversible": "ALLOW"}}
	for k, v := range in {
		base[k] = v
	}
	return f.live(t, f.register(t, base)), q
}

func (f fixture) start(t *testing.T, a domain.Agent, target shared.ID, q *queued) domain.Job {
	t.Helper()
	desk := f.desk
	desk.Queue = q
	out, err := (StartAgentJob{Desk: desk}).Descriptor().Handler.Invoke(context.Background(), person(ana),
		usecase.Input{"agent_id": a.ID.String(), "target_id": target.String()})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return f.store.jobs[shared.MustParseID(out.String("id"))]
}

func (f fixture) run(t *testing.T, job domain.Job, ws *workspace, proposer Proposer) domain.Job {
	t.Helper()
	desk := f.desk
	ws.gate = Gate{Desk: desk}
	desk.Catalogue = ws
	ended, err := Runner{Desk: desk, Proposer: proposer}.Run(context.Background(), tenantID, job.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return ended
}

// UC-AGT-07 checks 1 and 3: a job is started by the agent's owner, queued, and acts as the agent's
// own account; the CI suggestion proposes the CIs a ticket names and applies nothing, though its
// policy would allow the write.
func TestTheCiSuggestionProposesTheCisATicketNames(t *testing.T) {
	f := newFixture()
	a, q := f.shipped(t, "CI_SUGGESTION", nil)
	job := f.start(t, a, ticket, q)
	if job.Status != domain.JobRunning || len(q.requests) != 1 || q.requests[0].Kind != queue.KindAgentJob {
		t.Fatalf("job = %+v, queued %+v", job, q.requests)
	}
	ws := &workspace{}
	ended := f.run(t, job, ws, nil)
	if ended.Status != domain.JobDone || ended.Code != OutcomeJobProposed || ended.Proposals != 1 {
		t.Fatalf("ended = %+v", ended)
	}
	if len(ws.performed) != 0 {
		t.Fatalf("a shipped agent applied: %+v", ws.performed)
	}
	var held domain.Request
	for _, r := range f.store.requests {
		held = r
	}
	body, _ := held.Input["body"].(string)
	if held.UseCase != addCommentName || !strings.Contains(body, "hubtask:ci/"+printer.String()) ||
		!strings.Contains(body, "hubtask:ci/"+router.String()) || strings.Contains(body, "Scanner") {
		t.Fatalf("held = %+v", held)
	}
	for _, actor := range ws.actors {
		if actor.AccountID != a.AccountID || actor.OnBehalfOf != ana || actor.Kind != appshared.ActorAIAgent {
			t.Fatalf("acted as %+v", actor)
		}
	}
}

// UC-AGT-07 check 2: the triage's labels are a suggestion, the tokens it spent are in the job and
// the ledger, and a job beyond its runs at once is refused.
func TestTheTriageSpendsItsTokensIntoTheLedger(t *testing.T) {
	f := newFixture()
	a, q := f.shipped(t, "TICKET_TRIAGE", usecase.Input{"budget": map[string]any{"concurrent_runs": float64(1), "tokens_per_day": float64(100), "actions_per_hour": float64(50)}})
	job := f.start(t, a, ticket, q)
	desk := f.desk
	desk.Queue = q
	if _, err := (StartAgentJob{Desk: desk}).Descriptor().Handler.Invoke(context.Background(), person(ana),
		usecase.Input{"agent_id": a.ID.String(), "target_id": ticket.String()}); code(err) != "agents.budget_spent" {
		t.Fatalf("a second run at once: %v", err)
	}
	ended := f.run(t, job, &workspace{}, classifier{tokens: 120})
	if ended.Tokens != 120 || ended.Proposals != 2 {
		t.Fatalf("ended = %+v", ended)
	}
	var spent int64
	for _, action := range f.store.actions {
		if action.UseCase == runAgentJobName {
			spent += action.Tokens
		}
	}
	if spent != 120 {
		t.Fatalf("ledger tokens = %d", spent)
	}
	if _, err := (StartAgentJob{Desk: desk}).Descriptor().Handler.Invoke(context.Background(), person(ana),
		usecase.Input{"agent_id": a.ID.String(), "target_id": ticket.String()}); code(err) != "agents.budget_spent" ||
		shared.AsError(err).Params["budget"] != "tokens_per_day" {
		t.Fatalf("a run past the day's tokens: %v", err)
	}
}

// UC-AGT-07 check 4: the duplicate review proposes the CI a source saw last as the survivor; the
// knowledge draft proposes a page only from a resolved ticket.
func TestTheDuplicateReviewAndTheKnowledgeDraftPropose(t *testing.T) {
	f := newFixture()
	a, q := f.shipped(t, "DUPLICATE_REVIEW", nil)
	if ended := f.run(t, f.start(t, a, hubID, q), &workspace{}, nil); ended.Proposals != 1 {
		t.Fatalf("review = %+v", ended)
	}
	for _, r := range f.store.requests {
		if r.UseCase != resolveCiDuplicateName || r.Input["survivor_id"] != router.String() || r.Input["duplicate_id"] != dupID.String() {
			t.Fatalf("held = %+v", r)
		}
	}

	g := newFixture()
	k, kq := g.shipped(t, "KNOWLEDGE_DRAFT", nil)
	if ended := g.run(t, g.start(t, k, ticket, kq), &workspace{}, nil); ended.Code != OutcomeJobEntryOpen || ended.Proposals != 0 {
		t.Fatalf("an open ticket was drafted from: %+v", ended)
	}
	ended := g.run(t, g.start(t, k, ticket, kq), &workspace{completed: true}, nil)
	if ended.Proposals != 1 {
		t.Fatalf("draft = %+v", ended)
	}
	found := false
	for _, r := range g.store.requests {
		body, _ := r.Input["body"].(string)
		if r.UseCase == createPageName && r.Input["hub_id"] == hubID.String() && strings.Contains(body, "hubtask:entry/"+ticket.String()) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no page proposed: %+v", g.store.requests)
	}
}

// UC-AGT-07 check 5: an agent built elsewhere runs no job, a job of a stopped agent ends refused,
// and no agent starts a job.
func TestAJobIsRefusedWhereItMayNotRun(t *testing.T) {
	f := newFixture()
	ext := f.live(t, f.register(t, usecase.Input{"kind": "EXTERNAL"}))
	desk := f.desk
	desk.Queue = &queued{}
	if _, err := (StartAgentJob{Desk: desk}).Descriptor().Handler.Invoke(context.Background(), person(ana),
		usecase.Input{"agent_id": ext.ID.String(), "target_id": ticket.String()}); code(err) != "agents.runs_elsewhere" {
		t.Fatalf("external = %v", err)
	}
	a, q := f.shipped(t, "CI_SUGGESTION", nil)
	if _, err := (StartAgentJob{Desk: desk}).Descriptor().Handler.Invoke(context.Background(),
		appshared.ActorContext{Kind: appshared.ActorAIAgent, TenantID: tenantID, AccountID: ext.AccountID},
		usecase.Input{"agent_id": a.ID.String(), "target_id": ticket.String()}); code(err) != "agents.agent_starts_not" {
		t.Fatalf("an agent started a job: %v", err)
	}
	job := f.start(t, a, ticket, q)
	if _, err := (SuspendAgent{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(ana), usecase.Input{"agent_id": a.ID.String()}); err != nil {
		t.Fatal(err)
	}
	ws := &workspace{}
	if ended := f.run(t, job, ws, nil); ended.Status != domain.JobFailed || ended.Code != "agents.suspended" || len(ws.actors) != 0 {
		t.Fatalf("a stopped agent's job = %+v", ended)
	}
}

func TestTheSurvivorIsTheCiSeenLastElseTheOldest(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		cis  []usecase.Output
		want string
	}{
		{"one is no duplicate", []usecase.Output{{"id": "a"}}, ""},
		{"seen last", []usecase.Output{{"id": "a", "last_seen_at": old}, {"id": "b", "last_seen_at": old.Add(time.Hour)}}, "b"},
		{"seen beats unseen", []usecase.Output{{"id": "a", "created_at": old}, {"id": "b", "last_seen_at": old}}, "b"},
		{"the oldest", []usecase.Output{{"id": "a", "created_at": old.Add(time.Hour)}, {"id": "b", "created_at": old}}, "b"},
	}
	for _, c := range cases {
		if got := Survivor(c.cis); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}
