// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	agentrepo "github.com/Jersyfi/hubtask/core/application/repository/agent"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	admin    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000002")
	ana      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000003")
	ben      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000004")
	noon     = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
)

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// gate lets the administrator administer, and nobody else.
type gate struct{}

func (gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if r.TokenScope != "" {
		if err := actor.RequireScope(r.TokenScope); err != nil {
			return err
		}
	}
	if actor.AccountID == admin {
		return nil
	}
	return shared.ErrForbidden.WithDetail("access.not_permitted")
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, e audit.Entry) error {
	t.entries = append(t.entries, e)
	return nil
}

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-9000-%012d", i.n))
}

// store is the port in memory.
type store struct {
	models   map[shared.ID]domain.Model
	agents   map[shared.ID]domain.Agent
	halt     agentrepo.Halt
	actions  []domain.Action
	requests map[shared.ID]domain.Request
	cases    []domain.Case
	runs     []domain.Run
	marks    []domain.Mark
	jobs     map[shared.ID]domain.Job
}

func newStore() *store {
	return &store{models: map[shared.ID]domain.Model{}, agents: map[shared.ID]domain.Agent{}, requests: map[shared.ID]domain.Request{}, jobs: map[shared.ID]domain.Job{}}
}

func notFound() error { return shared.ErrNotFound.WithDetail("agents.not_found") }

func (s *store) ListModels(context.Context, int) ([]domain.Model, error) {
	out := []domain.Model{}
	for _, m := range s.models {
		out = append(out, m)
	}
	return out, nil
}
func (s *store) FindModel(_ context.Context, id shared.ID) (domain.Model, error) {
	if m, ok := s.models[id]; ok {
		return m, nil
	}
	return domain.Model{}, notFound()
}
func (s *store) InsertModel(_ context.Context, m domain.Model, _ time.Time) error {
	s.models[m.ID] = m
	return nil
}
func (s *store) UpdateModel(_ context.Context, m domain.Model, expected int, _ time.Time) error {
	if s.models[m.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	m.Version = expected + 1
	s.models[m.ID] = m
	return nil
}
func (s *store) ListAgents(_ context.Context, owner, model shared.ID, _ int) ([]domain.Agent, error) {
	out := []domain.Agent{}
	for _, a := range s.agents {
		if (owner.IsZero() || a.OwnerID == owner) && (model.IsZero() || a.ModelID == model) {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *store) FindAgent(_ context.Context, id shared.ID) (domain.Agent, error) {
	if a, ok := s.agents[id]; ok {
		return a, nil
	}
	return domain.Agent{}, notFound()
}
func (s *store) FindAgentByAccount(_ context.Context, account shared.ID) (domain.Agent, error) {
	for _, a := range s.agents {
		if a.AccountID == account {
			return a, nil
		}
	}
	return domain.Agent{}, notFound()
}
func (s *store) InsertAgent(_ context.Context, a domain.Agent) error { s.agents[a.ID] = a; return nil }
func (s *store) UpdateAgent(_ context.Context, a domain.Agent, expected int, _ time.Time) error {
	if s.agents[a.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	a.Version = expected + 1
	s.agents[a.ID] = a
	return nil
}
func (s *store) Halt(context.Context) (agentrepo.Halt, error) { return s.halt, nil }
func (s *store) PutHalt(_ context.Context, _ shared.ID, h agentrepo.Halt) error {
	s.halt = h
	return nil
}
func (s *store) RecordAction(_ context.Context, a domain.Action) error {
	s.actions = append(s.actions, a)
	return nil
}
func (s *store) Usage(_ context.Context, agent shared.ID, hourFrom, dayFrom time.Time) (domain.Usage, error) {
	var u domain.Usage
	for _, a := range s.actions {
		if a.AgentID != agent {
			continue
		}
		if !a.At.Before(hourFrom) && a.Class != domain.ClassRead && a.Decision != domain.DecisionRefused {
			u.ActionsLastHour++
		}
		if !a.At.Before(dayFrom) {
			u.TokensToday += a.Tokens
		}
		if !a.At.Before(hourFrom) && a.Decision == domain.DecisionNotified {
			u.NotifiedLastHour++
		}
	}
	return u, nil
}
func (s *store) ListActions(context.Context, shared.ID, *time.Time, int) ([]domain.Action, error) {
	return s.actions, nil
}
func (s *store) Metering(context.Context, shared.ID, time.Time, int) ([]agentrepo.Metering, error) {
	return nil, nil
}
func (s *store) InsertRequest(_ context.Context, r domain.Request) error {
	s.requests[r.ID] = r
	return nil
}
func (s *store) FindRequest(_ context.Context, id shared.ID) (domain.Request, error) {
	if r, ok := s.requests[id]; ok {
		return r, nil
	}
	return domain.Request{}, notFound()
}
func (s *store) ListRequests(context.Context, domain.RequestStatus, shared.ID, int) ([]domain.Request, error) {
	out := []domain.Request{}
	for _, r := range s.requests {
		out = append(out, r)
	}
	return out, nil
}
func (s *store) DecideRequest(_ context.Context, r domain.Request, expected int) (bool, error) {
	if s.requests[r.ID].Status != domain.RequestPending || s.requests[r.ID].Version != expected {
		return false, nil
	}
	r.Version = expected + 1
	s.requests[r.ID] = r
	return true, nil
}
func (s *store) FailRequest(_ context.Context, id shared.ID, code string) error {
	r := s.requests[id]
	r.Status, r.Code = domain.RequestFailed, code
	s.requests[id] = r
	return nil
}
func (s *store) ListCases(_ context.Context, agent shared.ID, _ int) ([]domain.Case, error) {
	out := []domain.Case{}
	for _, c := range s.cases {
		if c.AgentID == agent {
			out = append(out, c)
		}
	}
	return out, nil
}
func (s *store) InsertCase(_ context.Context, c domain.Case) error {
	s.cases = append(s.cases, c)
	return nil
}
func (s *store) DeleteCase(_ context.Context, agent, id shared.ID) (bool, error) {
	for i, c := range s.cases {
		if c.AgentID == agent && c.ID == id {
			s.cases = append(s.cases[:i], s.cases[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (s *store) InsertRun(_ context.Context, r domain.Run) error {
	s.runs = append([]domain.Run{r}, s.runs...)
	return nil
}
func (s *store) ListRuns(_ context.Context, agent shared.ID, limit int) ([]domain.Run, error) {
	out := []domain.Run{}
	for _, r := range s.runs {
		if r.AgentID == agent && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *store) InsertMark(_ context.Context, m domain.Mark) error {
	s.marks = append(s.marks, m)
	return nil
}
func (s *store) ListMarks(context.Context, string, shared.ID, int) ([]domain.Mark, error) {
	return s.marks, nil
}
func (s *store) InsertJob(_ context.Context, j domain.Job) error {
	s.jobs[j.ID] = j
	return nil
}
func (s *store) EndJob(_ context.Context, j domain.Job) (bool, error) {
	if was, ok := s.jobs[j.ID]; !ok || was.Status != domain.JobRunning {
		return false, nil
	}
	s.jobs[j.ID] = j
	return true, nil
}
func (s *store) FindJob(_ context.Context, id shared.ID) (domain.Job, error) {
	if j, ok := s.jobs[id]; ok {
		return j, nil
	}
	return domain.Job{}, notFound()
}
func (s *store) ListJobs(_ context.Context, agent shared.ID, limit int) ([]domain.Job, error) {
	out := []domain.Job{}
	for _, j := range s.jobs {
		if j.AgentID == agent && len(out) < limit {
			out = append(out, j)
		}
	}
	return out, nil
}
func (s *store) RunningJobs(_ context.Context, agent shared.ID, since time.Time) (int, error) {
	n := 0
	for _, j := range s.jobs {
		if j.AgentID == agent && j.Status == domain.JobRunning && !j.StartedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

// catalogue creates a service account and records every other call with its actor.
type catalogue struct {
	ids   *ids
	calls []call
}

type call struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
}

func (c *catalogue) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.calls = append(c.calls, call{name, actor, in})
	if name == createServiceAccount {
		return usecase.Output{"id": c.ids.NewID().String()}, nil
	}
	return usecase.Output{"ok": true}, nil
}

type fixture struct {
	desk  Desk
	store *store
	cat   *catalogue
	trail *trail
}

func newFixture() fixture {
	gen := &ids{}
	s := newStore()
	cat := &catalogue{ids: gen}
	t := &trail{}
	return fixture{
		desk: Desk{Store: s, Catalogue: cat, Authorizer: gate{}, Audit: t, UnitOfWork: inline{},
			Clock: clock.Fixed(noon), IDs: gen},
		store: s, cat: cat, trail: t,
	}
}

func person(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: id,
		Scopes: []string{scopeRead, scopeWrite, "members:write", "items:read", "items:write"},
	}
}

func (f fixture) register(t *testing.T, in usecase.Input) domain.Agent {
	t.Helper()
	base := usecase.Input{"name": "Triage", "purpose": "Sorts new tickets.", "kind": "TICKET_TRIAGE", "owner_id": ana.String()}
	for k, v := range in {
		base[k] = v
	}
	out, err := RegisterAgent{Desk: f.desk}.Descriptor().Handler.Invoke(context.Background(), person(admin), base)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	id := shared.MustParseID(out.String("id"))
	return f.store.agents[id]
}

func (f fixture) live(t *testing.T, a domain.Agent) domain.Agent {
	t.Helper()
	ctx := context.Background()
	in := usecase.Input{"agent_id": a.ID.String()}
	if _, err := (ApproveAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), in); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := (ActivateAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), in); err != nil {
		t.Fatalf("activate: %v", err)
	}
	return f.store.agents[a.ID]
}

func code(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}

// UC-AGT-01 checks 1 and 2: an agent is registered as a draft with a service account of its own,
// and its owner does not approve it.
func TestAnAgentIsRegisteredAsADraftOnItsOwnAccount(t *testing.T) {
	f := newFixture()
	a := f.register(t, nil)
	if a.Status != domain.StatusDraft || a.AccountID.IsZero() || a.OwnerID != ana {
		t.Fatalf("agent = %+v", a)
	}
	if len(f.cat.calls) != 1 || f.cat.calls[0].name != createServiceAccount {
		t.Fatalf("calls = %+v", f.cat.calls)
	}
	if _, err := (RegisterAgent{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(ben),
		usecase.Input{"name": "x", "purpose": "y", "kind": "EXTERNAL"}); code(err) != "access.not_permitted" {
		t.Fatalf("a member registered an agent: %v", err)
	}
	owner := person(ana)
	if _, err := (ApproveAgent{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), owner,
		usecase.Input{"agent_id": a.ID.String()}); err == nil {
		t.Fatal("its owner approved it")
	}
	if _, err := (RegisterAgent{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(admin),
		usecase.Input{"name": "x", "purpose": "y", "kind": "EXTERNAL", "risk_class": "HIGH"}); code(err) != "agents.high_risk_not_registrable" {
		t.Fatalf("a high-risk agent was registered: %v", err)
	}
}

// UC-AGT-02 checks 1, 3 and 4: a draft's acts are refused; a live agent's read runs as the person
// it acts for with its scopes narrowed; a write waits for a person and is recorded with its input.
func TestTheGateHoldsAnAgentToItsGovernance(t *testing.T) {
	f := newFixture()
	a := f.register(t, usecase.Input{"scopes": []any{"items:read", "items:write"}})
	var seen appshared.ActorContext
	read := usecase.Descriptor{Name: "GetWorkItem", ReadOnly: true}
	write := usecase.Descriptor{Name: "UpdateWorkItem"}
	run := func(d usecase.Descriptor, in usecase.Input) (usecase.Output, error) {
		next := usecase.HandlerFunc(func(_ context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			seen = actor
			return usecase.Output{}, nil
		})
		actor := appshared.ActorContext{
			Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: a.AccountID,
			Scopes: []string{"items:read", "items:write", "members:write"},
		}
		return Gate{Desk: f.desk}.Wrap(nil)(d, next).Invoke(context.Background(), actor, in)
	}

	if _, err := run(read, usecase.Input{}); code(err) != "agents.not_active" {
		t.Fatalf("a draft read: %v", err)
	}
	f.live(t, a)
	if _, err := run(read, usecase.Input{}); err != nil {
		t.Fatalf("a live agent's read: %v", err)
	}
	if seen.OnBehalfOf != ana || seen.AsAgent != a.ID || seen.Kind != appshared.ActorAIAgent || slices.Contains(seen.Scopes, "members:write") {
		t.Fatalf("the read ran as %+v", seen)
	}
	_, err := run(write, usecase.Input{"item_id": "x", "title": "New", "step_up_token": "secret"})
	if code(err) != "agents.approval_required" {
		t.Fatalf("a write ran without a person: %v", err)
	}
	requestID := shared.MustParseID(shared.AsError(err).Params["request_id"])
	held := f.store.requests[requestID]
	if held.UseCase != "UpdateWorkItem" || held.Input["title"] != "New" || held.Input["step_up_token"] != nil {
		t.Fatalf("held = %+v", held)
	}
	last := f.store.actions[len(f.store.actions)-1]
	if last.Decision != domain.DecisionHeld || last.RequestID != requestID {
		t.Fatalf("ledger = %+v", last)
	}

	// UC-AGT-02 check 5: approving performs it as the approving person.
	out, err := (DecideAgentRequest{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(ana),
		usecase.Input{"request_id": requestID.String(), "approve": true})
	if err != nil || out["status"] != "APPROVED" {
		t.Fatalf("approve = %v %v", out, err)
	}
	performed := f.cat.calls[len(f.cat.calls)-1]
	if performed.name != "UpdateWorkItem" || performed.actor.AccountID != ana || performed.in["title"] != "New" {
		t.Fatalf("performed = %+v", performed)
	}
	if _, err := (DecideAgentRequest{Desk: f.desk}).Descriptor().Handler.Invoke(context.Background(), person(admin),
		usecase.Input{"request_id": requestID.String(), "approve": true}); code(err) != "agents.request_decided" {
		t.Fatalf("a second approval performed it again: %v", err)
	}
}

// UC-AGT-02 checks 6 and 7: the agent's kill switch and the workspace's stop it at once; a spent
// budget refuses its writes with when it renews.
func TestTheKillSwitchesAndTheBudgetStopAnAgent(t *testing.T) {
	f := newFixture()
	a := f.register(t, usecase.Input{
		"kind": "EXTERNAL", "policy": map[string]any{"reversible": "ALLOW"}, "budget": map[string]any{"actions_per_hour": float64(1)},
	})
	f.live(t, a)
	write := usecase.Descriptor{Name: "UpdateWorkItem"}
	actor := appshared.ActorContext{Kind: appshared.ActorAIAgent, TenantID: tenantID, AccountID: a.AccountID}
	run := func() error {
		next := usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, nil
		})
		_, err := Gate{Desk: f.desk}.Wrap(nil)(write, next).Invoke(context.Background(), actor, usecase.Input{})
		return err
	}
	if err := run(); err != nil {
		t.Fatalf("an allowed write: %v", err)
	}
	err := run()
	if code(err) != "agents.budget_spent" || shared.AsError(err).Params["renews_at"] != "2026-11-02T13:00:00Z" {
		t.Fatalf("over budget: %v", err)
	}

	ctx := context.Background()
	if _, err := (SuspendAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(ana), usecase.Input{"agent_id": a.ID.String()}); err != nil {
		t.Fatalf("its owner could not stop it: %v", err)
	}
	if err := run(); code(err) != "agents.suspended" {
		t.Fatalf("a suspended agent acted: %v", err)
	}
	if _, err := (SetAgentHalt{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{"halted": true}); err != nil {
		t.Fatal(err)
	}
	if err := run(); code(err) != "agents.halted" {
		t.Fatalf("a halted workspace's agent acted: %v", err)
	}
}

// UC-AGT-03 checks 3 and 4: a changed model is a new revision, and an agent using it is refused
// until it is approved again.
func TestAChangedModelStopsItsAgentsUntilApprovedAgain(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	out, err := (RegisterAgentModel{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"key": "local", "name": "Local", "source": "OWN", "model": "m1", "region": "SELF_HOSTED",
	})
	if err != nil {
		t.Fatal(err)
	}
	model := out.String("id")
	a := f.live(t, f.register(t, usecase.Input{"model_id": model, "policy": map[string]any{"reversible": "ALLOW"}}))
	if _, err := (ChangeAgentModel{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
		"model_id": model, "name": "Local", "source": "OWN", "model": "m2", "region": "SELF_HOSTED",
	}); err != nil {
		t.Fatal(err)
	}
	actor := appshared.ActorContext{Kind: appshared.ActorAIAgent, TenantID: tenantID, AccountID: a.AccountID}
	next := usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
		return usecase.Output{}, nil
	})
	if _, err := (Gate{Desk: f.desk}).Wrap(nil)(usecase.Descriptor{Name: "X", ReadOnly: true}, next).Invoke(ctx, actor, nil); code(err) != "agents.model_changed" {
		t.Fatalf("an agent on a changed model acted: %v", err)
	}
	read, _ := (ReadAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{"agent_id": a.ID.String()})
	if read["needs_approval"] != true {
		t.Fatalf("read = %v", read)
	}
}

// UC-AGT-04 checks 1 … 3: an agent asking for an evaluation goes live only after a run on its
// model reached its bar; the server scores the answers, and the agent answers its set itself.
func TestAnAgentGoesLiveOnlyAfterPassingItsEvaluation(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	a := f.register(t, usecase.Input{"min_accuracy": float64(50)})
	for _, title := range []string{"Printer jams", "VPN down"} {
		if _, err := (AddAgentCase{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), usecase.Input{
			"agent_id": a.ID.String(), "title": title, "expected": map[string]any{"labels": []any{"hw"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	in := usecase.Input{"agent_id": a.ID.String()}
	if _, err := (ApproveAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), in); err != nil {
		t.Fatal(err)
	}
	if _, err := (ActivateAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), in); code(err) != "agents.evaluation_required" {
		t.Fatalf("an unevaluated agent went live: %v", err)
	}
	// The agent itself answers its set through the gate, before it is live.
	itself := appshared.ActorContext{Kind: appshared.ActorAIAgent, TenantID: tenantID, AccountID: a.AccountID, Scopes: []string{scopeRead, scopeWrite}}
	f.store.agents[a.ID] = func() domain.Agent { x := f.store.agents[a.ID]; x.Scopes = []string{scopeRead, scopeWrite}; return x }()
	d := RecordAgentEvaluation{Desk: f.desk}.Descriptor()
	handler := (Gate{Desk: f.desk}).Wrap(nil)(d, d.Handler)
	answers := []any{map[string]any{"case_id": f.store.cases[0].ID.String(), "answer": map[string]any{"labels": []any{"hw"}}}}
	out, err := handler.Invoke(ctx, itself, usecase.Input{"agent_id": a.ID.String(), "answers": answers})
	if err != nil || out["passed"] != 1 || out["cases"] != 2 || out["accuracy"] != 50 {
		t.Fatalf("run = %v %v", out, err)
	}
	if _, err := (ActivateAgent{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), in); err != nil {
		t.Fatalf("a passing agent did not go live: %v", err)
	}
}

// UC-AGT-02 check 8: an act of no registered agent passes untouched, and so does a person's.
func TestTheGateLetsEveryOtherActorThrough(t *testing.T) {
	f := newFixture()
	ran := 0
	next := usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
		ran++
		return usecase.Output{}, nil
	})
	h := (Gate{Desk: f.desk}).Wrap(nil)(usecase.Descriptor{Name: "UpdateWorkItem"}, next)
	for _, actor := range []appshared.ActorContext{
		person(ana),
		{Kind: appshared.ActorServiceAccount, TenantID: tenantID, AccountID: ben},
	} {
		if _, err := h.Invoke(context.Background(), actor, usecase.Input{}); err != nil {
			t.Fatal(err)
		}
	}
	if ran != 2 || len(f.store.actions) != 0 {
		t.Fatalf("ran %d, ledger %d", ran, len(f.store.actions))
	}
}

// UC-AGT-04 check 7: a shipped agent answers its own set from each case's text - a knowledge draft
// its title and body, a CI suggestion the CIs the text names - and the server scores the run; a
// duplicate review is evaluated by answers somebody sends, and a CI suggestion needs its hub.
func TestAShippedAgentAnswersItsOwnSet(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	draft := f.register(t, usecase.Input{"kind": "KNOWLEDGE_DRAFT"})
	for _, c := range []usecase.Input{
		{"title": "Reset the VPN", "notes": "Restart the client.", "expected": map[string]any{"title": "Reset the VPN", "body": "Restart the client."}},
		{"title": "Printer jams", "expected": map[string]any{"title": "Something else"}},
	} {
		c["agent_id"] = draft.ID.String()
		if _, err := (AddAgentCase{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), c); err != nil {
			t.Fatal(err)
		}
	}
	run := RunAgentEvaluation{Desk: f.desk}.Descriptor().Handler
	out, err := run.Invoke(ctx, person(admin), usecase.Input{"agent_id": draft.ID.String()})
	if err != nil || out["cases"] != 2 || out["passed"] != 1 || out["accuracy"] != 50 {
		t.Fatalf("run = %v %v", out, err)
	}
	if _, err := run.Invoke(ctx, person(ana), usecase.Input{"agent_id": draft.ID.String()}); err == nil {
		t.Fatal("somebody who administers no agent ran an evaluation")
	}

	cis := f.register(t, usecase.Input{"kind": "CI_SUGGESTION"})
	if _, err := run.Invoke(ctx, person(admin), usecase.Input{"agent_id": cis.ID.String()}); code(err) != "agents.evaluation_hub_required" {
		t.Fatalf("a CI suggestion without its hub = %v", err)
	}
	dup := f.register(t, usecase.Input{"kind": "DUPLICATE_REVIEW"})
	if _, err := run.Invoke(ctx, person(admin), usecase.Input{"agent_id": dup.ID.String()}); code(err) != "agents.evaluation_by_answers" {
		t.Fatalf("a duplicate review = %v", err)
	}

	answers := AnswerCases(domain.KindCISuggestion, []domain.Case{
		{ID: "c1", Title: "The printer-3 on floor 3 is down", Notes: "and Router-2 blinks"},
		{ID: "c2", Title: "Nothing named"},
	}, []string{"Printer-3", "Router-2", "Switch"})
	if got := answers["c1"]["cis"]; !domain.Matches(map[string]any{"cis": []any{"Router-2", "Printer-3"}}, map[string]any{"cis": got}) {
		t.Fatalf("named = %v", got)
	}
	if got, _ := answers["c2"]["cis"].([]any); len(got) != 0 {
		t.Fatalf("a text naming nothing answered %v", got)
	}
}

type labeller struct {
	asked      []string
	collection shared.ID
}

func (l *labeller) Labels(_ context.Context, _ appshared.ActorContext, collection shared.ID, title, _ string) ([]string, int64, error) {
	l.asked, l.collection = append(l.asked, title), collection
	return []string{"Hardware"}, 40, nil
}

// UC-AGT-04 check 8: a triage given a collection answers the labels the classifier chooses for each
// case expecting labels, and only those; without a collection none are answered and those cases
// fail.
func TestATriageAnswersTheLabelsOfItsOwnSet(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	triage := f.register(t, usecase.Input{"kind": "TICKET_TRIAGE"})
	for _, c := range []usecase.Input{
		{"title": "Printer jams", "expected": map[string]any{"labels": []any{"Hardware"}}},
		{"title": "VPN drops", "expected": map[string]any{"labels": []any{"Network"}}},
		{"title": "Nothing named", "expected": map[string]any{"cis": []any{}}},
	} {
		c["agent_id"] = triage.ID.String()
		if _, err := (AddAgentCase{Desk: f.desk}).Descriptor().Handler.Invoke(ctx, person(admin), c); err != nil {
			t.Fatal(err)
		}
	}
	l := &labeller{}
	run := RunAgentEvaluation{Desk: f.desk, Labels: l}.Descriptor().Handler
	out, err := run.Invoke(ctx, person(admin), usecase.Input{
		"agent_id": triage.ID.String(), "hub_id": hubID.String(), "collection_id": collection.String(),
	})
	if err != nil || out["cases"] != 3 || out["passed"] != 2 {
		t.Fatalf("run = %v %v", out, err)
	}
	if len(l.asked) != 2 || l.collection != collection {
		t.Fatalf("asked %v about %s", l.asked, l.collection)
	}
	out, err = run.Invoke(ctx, person(admin), usecase.Input{"agent_id": triage.ID.String(), "hub_id": hubID.String()})
	if err != nil || out["passed"] != 1 || len(l.asked) != 2 {
		t.Fatalf("without a collection = %v %v, asked %v", out, err, l.asked)
	}
	if _, err := run.Invoke(ctx, person(admin), usecase.Input{
		"agent_id": triage.ID.String(), "hub_id": hubID.String(), "collection_id": "nope",
	}); err == nil {
		t.Fatalf("an unreadable collection = %v", err)
	}
}
