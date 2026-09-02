// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// runLog is the run repository in memory.
type runLog struct {
	rows    map[shared.ID]domain.Run
	order   []shared.ID
	counted int
	since   int
}

func newRunLog() *runLog { return &runLog{rows: map[shared.ID]domain.Run{}} }

func (l *runLog) Start(_ context.Context, run domain.Run) error {
	l.rows[run.ID] = run
	l.order = append(l.order, run.ID)
	return nil
}

func (l *runLog) Finish(_ context.Context, run domain.Run) error {
	l.rows[run.ID] = run
	return nil
}

func (l *runLog) Find(_ context.Context, id shared.ID) (domain.Run, error) {
	run, found := l.rows[id]
	if !found {
		return domain.Run{}, shared.ErrNotFound.WithDetail("automation.run_not_found")
	}
	return run, nil
}

func (l *runLog) List(context.Context, repository.RunQuery) (repository.RunPage, error) {
	return repository.RunPage{}, nil
}

func (l *runLog) CountSince(context.Context, shared.ID, time.Time) (int, error) {
	l.counted++
	return l.since, nil
}

func (l *runLog) last() domain.Run {
	if len(l.order) == 0 {
		return domain.Run{}
	}
	return l.rows[l.order[len(l.order)-1]]
}

// failures is the consecutive-failure counter.
type failures struct {
	count     int
	cleared   int
	disabled  bool
	threshold int
}

func (f *failures) Bump(context.Context, shared.ID, time.Time) (int, error) {
	f.count++
	return f.count, nil
}

func (f *failures) Clear(context.Context, shared.ID, time.Time) error {
	f.cleared++
	f.count = 0
	return nil
}

func (f *failures) Disable(_ context.Context, _ shared.ID, threshold int, _ time.Time) (bool, error) {
	f.threshold = threshold
	if f.disabled {
		return false, nil
	}
	f.disabled = true
	return true, nil
}

// dispatched records every action the engine performed, and refuses the ones a test asks it to.
type dispatched struct {
	calls   []dispatchCall
	refuse  map[string]error
	scopes  map[string]string
	unknown bool
}

type dispatchCall struct {
	kind     string
	actor    appshared.ActorContext
	params   map[string]any
	supplied map[string]any
}

func newDispatched() *dispatched {
	return &dispatched{
		refuse: map[string]error{},
		scopes: map[string]string{"ADD_LABEL": "items:write", "CREATE_BUCKET": "containers:write"},
	}
}

func (d *dispatched) Dispatch(
	_ context.Context, runAs appshared.ActorContext, kind string,
	params map[string]any, supplied map[string]any,
) (usecase.Output, error) {
	d.calls = append(d.calls, dispatchCall{kind: kind, actor: runAs, params: params, supplied: supplied})
	if err, refused := d.refuse[kind]; refused {
		return nil, err
	}
	return usecase.Output{}, nil
}

func (d *dispatched) ForAction(kind string) (string, bool) {
	if d.unknown {
		return "", false
	}
	scope, known := d.scopes[kind]
	return scope, known
}

// claims is the idempotency guard: a key claimed once is not claimed again.
type claims struct{ seen map[string]bool }

func newClaims() *claims { return &claims{seen: map[string]bool{}} }

func (c *claims) Claim(_ context.Context, _ appshared.ActorContext, key string) (bool, error) {
	if c.seen[key] {
		return false, nil
	}
	c.seen[key] = true
	return true, nil
}

func (c *claims) Release(_ context.Context, _ appshared.ActorContext, key string) error {
	delete(c.seen, key)
	return nil
}

// published records the run events.
type published struct{ envelopes []event.Envelope }

func (p *published) Append(_ context.Context, envelope event.Envelope) error {
	p.envelopes = append(p.envelopes, envelope)
	return nil
}

func (p *published) types() []event.Type {
	var out []event.Type
	for _, envelope := range p.envelopes {
		out = append(out, envelope.Type)
	}
	return out
}

// source answers the event a run is about.
type source struct{ envelope event.Envelope }

func (s source) FindEvent(_ context.Context, id shared.ID) (event.Envelope, error) {
	if s.envelope.ID != id {
		return event.Envelope{}, shared.ErrNotFound.WithDetail("events.event_not_found")
	}
	return s.envelope, nil
}

// told records that a rule's owner was notified.
type told struct{ rules []shared.ID }

func (t *told) RuleDisabled(_ context.Context, rule domain.Rule, _ time.Time) error {
	t.rules = append(t.rules, rule.ID)
	return nil
}

// queued records what the engine parks on the queue: a WAIT's resume.
type queued struct{ requests []queue.Request }

func (q *queued) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	return shared.ID("01936f2a-7c1e-7000-8000-0000000000aa"), nil
}

type engineHarness struct {
	engine     RunRule
	rules      *ruleStore
	runs       *runLog
	failures   *failures
	dispatcher *dispatched
	events     *published
	owners     *told
	claims     *claims
	jobs       *queued
	signals    *runSignals
}

// runSignals records what the engine reports to the metrics (§4).
type runSignals struct {
	runs     []string // "result/trigger_type"
	disabled []string
}

func (s *runSignals) RuleRun(_ context.Context, result, triggerType string) {
	s.runs = append(s.runs, result+"/"+triggerType)
}

func (s *runSignals) RuleDisabled(_ context.Context, reason string) {
	s.disabled = append(s.disabled, reason)
}

func newEngine(t *testing.T, rule domain.Rule) *engineHarness {
	t.Helper()

	store := newRuleStore(rule)
	h := &engineHarness{
		rules: store, runs: newRunLog(), failures: &failures{},
		dispatcher: newDispatched(), events: &published{}, owners: &told{}, claims: newClaims(),
		jobs: &queued{}, signals: &runSignals{},
	}
	h.engine = RunRule{
		Rules: store, Runs: h.runs, Failures: h.failures, Events: h.events,
		Source:     source{envelope: itemEvent()},
		Dispatcher: h.dispatcher, Scopes: h.dispatcher,
		Conditions: compiler{}, Guard: h.claims, Owners: h.owners, Jobs: h.jobs,
		Signals:    h.signals,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: runIDs{},
	}
	return h
}

// runIDs mints a fresh identifier per run, so two runs in one test are two rows.
type runIDs struct{}

var minted int

func (runIDs) NewID() shared.ID {
	minted++
	return shared.ID("01936f2a-7c1e-7000-8000-0000000004" + string(rune('0'+minted%10)) + "0")
}

func enabledRule() domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Enabled = true
	rule.OnError = domain.OnErrorStop
	return rule
}

func engineActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}
}

func command(depth int) Command {
	return Command{
		RuleID: ruleID, Trigger: domain.TriggerEvent,
		EventID: itemEvent().ID, CausationDepth: depth,
	}
}

// The acceptance criterion: an event matching an enabled rule produces a run whose condition
// results and action outcomes are readable.
func TestAMatchingEventProducesARunWithItsResults(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}},
		{Kind: "CREATE_BUCKET", Params: map[string]any{"name": "Doing"}},
	}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("status %q, want SUCCEEDED", run.Status)
	}
	if len(run.ActionResults) != 2 {
		t.Fatalf("%d action results, want two", len(run.ActionResults))
	}
	for i, result := range run.ActionResults {
		if result.Status != domain.ActionSucceeded {
			t.Errorf("action %d is %q", i, result.Status)
		}
		if result.IdempotencyKey == "" {
			t.Errorf("action %d carries no idempotency key", i)
		}
	}
	if len(h.dispatcher.calls) != 2 {
		t.Errorf("%d actions dispatched", len(h.dispatcher.calls))
	}
	// The row is written before the conditions and finished afterwards, so what is stored is what
	// the caller was answered.
	if stored := h.runs.last(); stored.Status != domain.RunSucceeded {
		t.Errorf("the stored run says %q", stored.Status)
	}
}

// The run's actor is the service account with its real rights, checked by the authoriser exactly as
// a person's would be - and granted exactly the scope the action declares, no more.
func TestEveryActionIsPerformedAsTheRulesAccount(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}, {Kind: "CREATE_BUCKET"}}
	h := newEngine(t, rule)

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}

	wanted := map[string]string{"ADD_LABEL": "items:write", "CREATE_BUCKET": "containers:write"}
	for _, call := range h.dispatcher.calls {
		if call.actor.AccountID != serviceID {
			t.Errorf("%s ran as %s, want the rule's account", call.kind, call.actor.AccountID)
		}
		if call.actor.Kind != appshared.ActorServiceAccount {
			t.Errorf("%s ran as a %q", call.kind, call.actor.Kind)
		}
		if len(call.actor.Scopes) != 1 || call.actor.Scopes[0] != wanted[call.kind] {
			t.Errorf("%s was granted %v, want just %q", call.kind, call.actor.Scopes, wanted[call.kind])
		}
		// The label is read where the trail is written, not carried here - a rule that cached one
		// would record a name a release out of date.
		if call.actor.AccountName != "" {
			t.Errorf("%s carried the account name %q", call.kind, call.actor.AccountName)
		}
	}
}

// A `run_as` account lacking a right fails that action with the authoriser's own refusal on the run.
func TestARefusedActionCarriesTheAuthorisersOwnCode(t *testing.T) {
	rule := enabledRule()
	rule.OnError = domain.OnErrorContinue
	rule.Actions = []domain.Action{{Kind: "CREATE_BUCKET"}, {Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.refuse["CREATE_BUCKET"] = shared.ErrForbidden.WithDetail("access.not_permitted")

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.ActionResults[0].ErrorCode != "access.not_permitted" {
		t.Errorf("the refusal was rewritten to %q", run.ActionResults[0].ErrorCode)
	}
	// CONTINUE ran the rest, and the run did what its rule says.
	if run.ActionResults[1].Status != domain.ActionSucceeded {
		t.Errorf("CONTINUE did not continue: %+v", run.ActionResults[1])
	}
	if run.Status != domain.RunSucceeded {
		t.Errorf("status %q, want SUCCEEDED under CONTINUE", run.Status)
	}
}

// STOP stops, and the actions after the failure are SKIPPED rather than succeeded.
func TestOnErrorStopStopsAndSkipsTheRest(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "CREATE_BUCKET"}, {Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.refuse["CREATE_BUCKET"] = shared.ErrForbidden.WithDetail("access.not_permitted")

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunFailed {
		t.Errorf("status %q, want FAILED under STOP", run.Status)
	}
	if run.ActionResults[1].Status != domain.ActionSkipped {
		t.Errorf("the second action is %q, want SKIPPED", run.ActionResults[1].Status)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d actions were dispatched after a STOP", len(h.dispatcher.calls))
	}
}

// A redelivered event produces no second effect.
func TestARedeliveredEventDoesNotActTwice(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	for range 2 {
		if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
			t.Fatalf("running: %v", err)
		}
	}

	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d actions dispatched over two deliveries of one event", len(h.dispatcher.calls))
	}
	// The second run is recorded all the same: it happened, and it did nothing because the first
	// one had already acted.
	if len(h.runs.order) != 2 {
		t.Errorf("%d runs recorded, want one per delivery", len(h.runs.order))
	}
	if h.runs.last().Status != domain.RunSucceeded {
		t.Errorf("the second run says %q", h.runs.last().Status)
	}
}

// The index rather than the kind, because a rule may name one kind twice - a key that collapsed
// them would perform the first and silently skip the second.
func TestTheIdempotencyKeyDistinguishesTwoActionsOfOneKind(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}, {Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.ActionResults[0].IdempotencyKey == run.ActionResults[1].IdempotencyKey {
		t.Fatal("two actions of one kind share a key")
	}
	if len(h.dispatcher.calls) != 2 {
		t.Errorf("%d actions dispatched, want both", len(h.dispatcher.calls))
	}
	for _, result := range run.ActionResults {
		if !strings.Contains(result.IdempotencyKey, ruleID.String()) ||
			!strings.Contains(result.IdempotencyKey, itemEvent().ID.String()) {
			t.Errorf("the key %q does not name the rule and the event", result.IdempotencyKey)
		}
	}
}

// A chain that reaches the limit aborts, does nothing, and says which of the two things happened.
func TestARunAtTheDepthLimitAbortsAndActsOnNothing(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(),
		command(domain.MaxCausationDepth))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunAbortedLoop {
		t.Fatalf("status %q, want ABORTED_LOOP", run.Status)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Error("an aborted run acted")
	}
	// Evaluating conditions is where the reads are, and a loop that read on every hop would cost
	// exactly what the bound exists to prevent.
	if h.runs.counted != 0 {
		t.Error("an aborted run asked the throttle")
	}
	if run.ErrorCode != "automation.loop_depth_exceeded" {
		t.Errorf("code %q", run.ErrorCode)
	}
}

// The throttle holds a rule to its bound, and does it before the conditions are asked.
func TestTheThrottleHoldsARuleToItsBound(t *testing.T) {
	rule := enabledRule()
	rule.Throttle = domain.Throttle{MaxRunsPerHour: 10}
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.runs.since = 11

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunThrottled {
		t.Fatalf("status %q, want THROTTLED", run.Status)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Error("a throttled run acted")
	}
	if len(run.ConditionResults) != 0 {
		t.Error("a throttled run evaluated conditions")
	}
}

// The run being decided is already in the log, so the bound is reached when the count exceeds it -
// off by one the other way would let a rule bounded at one never run at all.
func TestARuleBoundedAtOneStillRunsOnce(t *testing.T) {
	rule := enabledRule()
	rule.Throttle = domain.Throttle{MaxRunsPerHour: 1}
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.runs.since = 1

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status == domain.RunThrottled {
		t.Error("a rule bounded at one was throttled on its first run")
	}
}

// A rule with no bound never asks: the count is a query, and a rule that does not throttle should
// not pay for one.
func TestARuleWithNoBoundNeverAsksTheThrottle(t *testing.T) {
	h := newEngine(t, enabledRule())

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}
	if h.runs.counted != 0 {
		t.Errorf("the throttle was asked %d times for a rule with no bound", h.runs.counted)
	}
}

// Every condition is answered, not up to the first false: a run log that stopped at the first no
// would answer "why did this not happen" with one line where somebody wants the whole picture.
func TestEveryConditionIsAnsweredAndOneFalseSkipsTheRun(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{
		{Expr: "event.type != ''"},
		{Expr: "false"},
		{Expr: "event.id != ''"},
	}
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSkipped {
		t.Fatalf("status %q, want SKIPPED", run.Status)
	}
	if len(run.ConditionResults) != 3 {
		t.Fatalf("%d condition results, want all three", len(run.ConditionResults))
	}
	if run.ConditionResults[1].Matched {
		t.Error("the false condition reports itself matched")
	}
	if len(h.dispatcher.calls) != 0 {
		t.Error("a skipped run acted")
	}
}

// A condition that could not be evaluated failed the run rather than skipping it: the rule did not
// decide anything, and the code says which condition and why.
func TestAConditionThatCannotBeEvaluatedFailsTheRun(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "event.type =="}}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunFailed {
		t.Fatalf("status %q, want FAILED", run.Status)
	}
	if run.ConditionResults[0].ErrorCode == "" {
		t.Error("the condition carries no code")
	}
	if run.ErrorCode != run.ConditionResults[0].ErrorCode {
		t.Errorf("the run says %q and the condition says %q",
			run.ErrorCode, run.ConditionResults[0].ErrorCode)
	}
}

// The failure counter disables the rule at its threshold and the owner is told.
func TestARuleThatKeepsFailingSwitchesItselfOffAndItsOwnerIsTold(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.refuse["ADD_LABEL"] = shared.ErrForbidden.WithDetail("access.not_permitted")

	for i := range MaxConsecutiveFailures {
		// A fresh event each time, so the idempotency guard does not swallow the repeat.
		cmd := command(0)
		cmd.EventID = shared.ID("01936f2a-7c1e-7000-8000-00000000060" + string(rune('0'+i)))
		if _, err := h.engine.Execute(context.Background(), engineActor(), cmd); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	if !h.failures.disabled {
		t.Fatalf("the rule was not switched off after %d failures", MaxConsecutiveFailures)
	}
	if h.failures.threshold != MaxConsecutiveFailures {
		t.Errorf("disabled at %d, want %d", h.failures.threshold, MaxConsecutiveFailures)
	}
	if len(h.owners.rules) != 1 || h.owners.rules[0] != ruleID {
		t.Errorf("the owner was told %v", h.owners.rules)
	}
}

// Every ended run reaches the metrics with its result and its trigger, and the self-protective
// switch-off reaches them with its reason - SLO-7 and A-16 have no other source. A WAITING run is
// not an end and must not be counted; its real end passes through settle later.
func TestTheMetricsSeeEveryEndedRunAndTheSwitchOff(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(h.signals.runs) != 1 || h.signals.runs[0] != "succeeded/event" {
		t.Errorf("the metrics saw %v, want [succeeded/event]", h.signals.runs)
	}

	h.dispatcher.refuse["ADD_LABEL"] = shared.ErrForbidden.WithDetail("access.not_permitted")
	for i := range MaxConsecutiveFailures {
		cmd := command(0)
		cmd.EventID = shared.ID("01936f2a-7c1e-7000-8000-00000000061" + string(rune('0'+i)))
		if _, err := h.engine.Execute(context.Background(), engineActor(), cmd); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := len(h.signals.runs); got != 1+MaxConsecutiveFailures {
		t.Errorf("the metrics saw %d runs, want %d", got, 1+MaxConsecutiveFailures)
	}
	if last := h.signals.runs[len(h.signals.runs)-1]; last != "failed/event" {
		t.Errorf("the last run reported %q, want failed/event", last)
	}
	if len(h.signals.disabled) != 1 || h.signals.disabled[0] != DisabledByStreak {
		t.Errorf("the switch-off reported %v, want [%s]", h.signals.disabled, DisabledByStreak)
	}
}

// Anything that is not a failure ends the streak - including a skip. A rule whose conditions said no
// is a rule that is working, and counting that towards being switched off would disable the most
// careful rules first.
func TestASkipEndsTheFailureStreak(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "false"}}
	h := newEngine(t, rule)
	h.failures.count = 3

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}
	if h.failures.cleared != 1 {
		t.Error("a skip did not end the streak")
	}
	if h.failures.disabled {
		t.Error("a skip switched the rule off")
	}
}

// The three events an engine emits, in the order they happen.
func TestARunEmitsItsStartAndItsEnd(t *testing.T) {
	t.Run("a run that acted", func(t *testing.T) {
		rule := enabledRule()
		rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
		h := newEngine(t, rule)

		if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
			t.Fatalf("running: %v", err)
		}
		want := []event.Type{event.RuleRunStarted, event.RuleRunFinished}
		if got := h.events.types(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("emitted %v, want %v", got, want)
		}
		finished := h.events.envelopes[1]
		if finished.Payload["status"] != string(domain.RunSucceeded) {
			t.Errorf("the finished event says %v", finished.Payload["status"])
		}
		// One hop further from the act a person performed, which is what makes the chain countable.
		if finished.CausationDepth != 1 {
			t.Errorf("depth %d, want one past the run's", finished.CausationDepth)
		}
	})

	t.Run("a run that failed", func(t *testing.T) {
		rule := enabledRule()
		rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
		h := newEngine(t, rule)
		h.dispatcher.refuse["ADD_LABEL"] = shared.ErrForbidden.WithDetail("access.not_permitted")

		if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
			t.Fatalf("running: %v", err)
		}
		want := []event.Type{event.RuleRunStarted, event.RuleRunFailed}
		if got := h.events.types(); len(got) != 2 || got[1] != want[1] {
			t.Fatalf("emitted %v, want %v", got, want)
		}
		if h.events.envelopes[1].Payload["rule_disabled"] != false {
			t.Error("one failure reported the rule as disabled")
		}
	})
}

// Deleted or disabled between the event and the run: there is no rule to record a run against, and a
// job for a rule nobody has is done.
func TestARuleThatWentAwayLeavesNoRun(t *testing.T) {
	for name, prepare := range map[string]func(*engineHarness){
		"deleted": func(h *engineHarness) { delete(h.rules.rows, ruleID) },
		"disabled": func(h *engineHarness) {
			rule := h.rules.rows[ruleID]
			rule.Enabled = false
			h.rules.rows[ruleID] = rule
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newEngine(t, enabledRule())
			prepare(h)

			if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
				t.Fatalf("running: %v", err)
			}
			if len(h.runs.order) != 0 {
				t.Errorf("%d runs recorded for a rule that is not there", len(h.runs.order))
			}
			if len(h.events.envelopes) != 0 {
				t.Error("events were emitted for a rule that is not there")
			}
		})
	}
}

// An action naming a kind this build does not serve fails that action rather than the process.
func TestAnActionThisBuildDoesNotServeFailsTheAction(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.unknown = true

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.ActionResults[0].ErrorCode != "automation.action_unknown" {
		t.Errorf("code %q", run.ActionResults[0].ErrorCode)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Error("an unknown action was dispatched anyway")
	}
}

// A run's own event is not something the run can be started by, and a run whose event was swept sees
// an empty one rather than failing: what the rule would have decided is no longer knowable.
func TestAnEventSweptWhileTheJobWaitedDoesNotFailTheRun(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "event.id != ''"}}
	h := newEngine(t, rule)

	cmd := command(0)
	cmd.EventID = shared.ID("01936f2a-7c1e-7000-8000-0000000007a0")

	run, err := h.engine.Execute(context.Background(), engineActor(), cmd)
	if err != nil {
		t.Fatalf("a swept event failed the run: %v", err)
	}
	if run.Status == domain.RunFailed {
		t.Errorf("status %q - a swept event is not a failure of the rule", run.Status)
	}
}

// The engine refuses to guess when it has no evaluator: a rule with conditions cannot be run at all,
// and running it as though there were none would act where the author said not to.
func TestARuleWithConditionsNeedsAnEngine(t *testing.T) {
	rule := enabledRule()
	rule.Conditions = []domain.Condition{{Expr: "true"}}
	h := newEngine(t, rule)
	h.engine.Conditions = nil

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error %v, want ErrInternal", err)
	}
}

// G-08. The run records which of the six triggers produced it, and who pulled it when a person did.
//
// On the run rather than resolved from the rule at read time: a rule can be edited from one kind
// into another, and a log that resolved the kind when somebody opened it would rewrite its own
// history.
func TestARunRecordsWhatStartedItAndWhoPulledIt(t *testing.T) {
	person := shared.ID("01936f2a-7c1e-7000-8000-00000000ac01")
	rule := enabledRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
	h := newEngine(t, rule)

	cmd := Command{
		RuleID: ruleID, Trigger: domain.TriggerManual,
		TriggeredBy: person, Occasion: "run/one",
	}
	run, err := h.engine.Execute(context.Background(), engineActor(), cmd)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Trigger != domain.TriggerManual {
		t.Errorf("the run says %q started it, want MANUAL", run.Trigger)
	}
	if run.TriggeredBy != person {
		t.Errorf("triggered by %q, want the person who pressed it", run.TriggeredBy)
	}
	if !run.EventID.IsZero() {
		t.Errorf("a manual run named event %q", run.EventID)
	}
	if h.runs.last().Trigger != domain.TriggerManual {
		t.Error("the stored run does not say what started it")
	}
}

// A job outlives the shape it was written for. A schedule pass queues a run, somebody edits the
// rule into an `EVENT` rule, and the job arrives: the producer no longer speaks for the rule, so
// nothing runs and nothing is recorded - a run against a trigger the rule no longer has would be a
// log entry for something that never happened.
func TestAJobWhoseTriggerTheRuleNoLongerHasDoesNothing(t *testing.T) {
	rule := enabledRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemCreated}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), Command{
		RuleID: ruleID, Trigger: domain.TriggerSchedule, Occasion: "2026-08-30T03:00:00Z",
	})
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != "" {
		t.Errorf("status %q, want no run at all", run.Status)
	}
	if len(h.runs.order) != 0 {
		t.Errorf("%d runs recorded, want none", len(h.runs.order))
	}
	if len(h.dispatcher.calls) != 0 {
		t.Errorf("%d actions performed, want none", len(h.dispatcher.calls))
	}
}

// The idempotency key names the occasion rather than the event, and this is why: five of the six
// triggers have no event, so a key derived from a zero event identifier would be the *same* key for
// every run of that rule for ever - the second press of a manual trigger would find the first's
// answer stored and do nothing at all.
func TestTwoOccasionsOfOneRuleDoNotShareAnIdempotencyKey(t *testing.T) {
	rule := enabledRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}}
	h := newEngine(t, rule)

	keys := map[string]bool{}
	for _, occasion := range []string{"run/one", "run/two"} {
		run, err := h.engine.Execute(context.Background(), engineActor(), Command{
			RuleID: ruleID, Trigger: domain.TriggerManual, Occasion: occasion,
		})
		if err != nil {
			t.Fatalf("running %s: %v", occasion, err)
		}
		if len(run.ActionResults) != 1 {
			t.Fatalf("%d action results for %s, want one", len(run.ActionResults), occasion)
		}
		keys[run.ActionResults[0].IdempotencyKey] = true
	}

	if len(keys) != 2 {
		t.Errorf("two presses shared one key %v - the second would silently do nothing", keys)
	}
	if len(h.dispatcher.calls) != 2 {
		t.Errorf("%d actions performed, want one per press", len(h.dispatcher.calls))
	}
}

// The engine's own guarantees do not belong to `EVENT`. Whatever started a run, the depth bound and
// the throttle answer the same way and the run is recorded either way - which is what "each
// producing into the engine G-07 built rather than a second execution path" means (G-08).
func TestEveryTriggerCarriesTheDepthBoundAndTheThrottle(t *testing.T) {
	kinds := []domain.TriggerKind{
		domain.TriggerEvent, domain.TriggerSchedule, domain.TriggerRelativeDate,
		domain.TriggerInboundWebhook, domain.TriggerManual, domain.TriggerJumbleEntry,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			rule := enabledRule()
			rule.Trigger = domain.Trigger{Kind: kind}
			if kind == domain.TriggerEvent {
				rule.Trigger.EventType = event.ItemCreated
			}
			cmd := Command{RuleID: ruleID, Trigger: kind, Occasion: "occasion/" + string(kind)}

			t.Run("depth", func(t *testing.T) {
				h := newEngine(t, rule)
				deep := cmd
				deep.CausationDepth = domain.MaxCausationDepth
				run, err := h.engine.Execute(context.Background(), engineActor(), deep)
				if err != nil {
					t.Fatalf("running: %v", err)
				}
				if run.Status != domain.RunAbortedLoop {
					t.Errorf("status %q at the depth limit, want ABORTED_LOOP", run.Status)
				}
				if len(h.dispatcher.calls) != 0 {
					t.Error("a run at the limit acted")
				}
				if h.runs.last().Trigger != kind {
					t.Errorf("the aborted run says %q started it", h.runs.last().Trigger)
				}
			})

			t.Run("throttle", func(t *testing.T) {
				throttled := rule
				throttled.Throttle = domain.Throttle{MaxRunsPerHour: 1}
				h := newEngine(t, throttled)
				h.runs.since = 2

				run, err := h.engine.Execute(context.Background(), engineActor(), cmd)
				if err != nil {
					t.Fatalf("running: %v", err)
				}
				if run.Status != domain.RunThrottled {
					t.Errorf("status %q past the bound, want THROTTLED", run.Status)
				}
				if len(h.dispatcher.calls) != 0 {
					t.Error("a throttled run acted")
				}
			})
		})
	}
}

// The flow vocabulary (G-09). A branch is a nested condition, a STOP is a deliberate end, and the
// run log names every action by its path.

func branchOf(condition string, then, otherwise []map[string]any) domain.Action {
	params := map[string]any{"condition": condition}
	arms := map[string][]map[string]any{"then": then, "else": otherwise}
	for name, actions := range arms {
		if actions == nil {
			continue
		}
		rows := make([]any, 0, len(actions))
		for _, action := range actions {
			rows = append(rows, map[string]any(action))
		}
		params[name] = rows
	}
	return domain.Action{Kind: domain.ActionBranch, Params: params}
}

// The acceptance criterion: BRANCH takes the branch its condition says, and the run log shows both
// the condition's answer and the path.
func TestABranchTakesTheArmItsConditionSays(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{branchOf("true",
		[]map[string]any{{"kind": "ADD_LABEL"}},
		[]map[string]any{{"kind": "CREATE_BUCKET"}},
	)}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("status %q, want SUCCEEDED", run.Status)
	}
	if len(run.ActionResults) != 2 {
		t.Fatalf("%d action results, want the branch and its arm: %+v", len(run.ActionResults), run.ActionResults)
	}

	branch := run.ActionResults[0]
	if branch.Kind != domain.ActionBranch || branch.Status != domain.ActionSucceeded {
		t.Errorf("the branch's own result is %+v", branch)
	}
	if branch.Path != "0" {
		t.Errorf("the branch's path is %q, want \"0\"", branch.Path)
	}
	if branch.Matched == nil || !*branch.Matched {
		t.Errorf("the log does not say the condition held: %+v", branch)
	}

	taken := run.ActionResults[1]
	if taken.Kind != "ADD_LABEL" || taken.Path != "0/then/0" {
		t.Errorf("the taken arm is %+v, want ADD_LABEL at 0/then/0", taken)
	}
	if len(h.dispatcher.calls) != 1 || h.dispatcher.calls[0].kind != "ADD_LABEL" {
		t.Errorf("dispatched %+v, want just the then arm", h.dispatcher.calls)
	}
}

// The arm the condition refuses is the else arm - and the log still shows the answer even though an
// empty arm leaves no nested results.
func TestABranchWhoseConditionSaysNoTakesElse(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{branchOf("false",
		[]map[string]any{{"kind": "ADD_LABEL"}},
		[]map[string]any{{"kind": "CREATE_BUCKET"}},
	)}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	branch := run.ActionResults[0]
	if branch.Matched == nil || *branch.Matched {
		t.Errorf("the log does not say the condition refused: %+v", branch)
	}
	if run.ActionResults[1].Path != "0/else/0" || run.ActionResults[1].Kind != "CREATE_BUCKET" {
		t.Errorf("the else arm is %+v", run.ActionResults[1])
	}
	if len(h.dispatcher.calls) != 1 || h.dispatcher.calls[0].kind != "CREATE_BUCKET" {
		t.Errorf("dispatched %+v, want just the else arm", h.dispatcher.calls)
	}
}

// STOP ends the run where it stands, the rest is SKIPPED, and the run succeeded: stopping early is
// what the rule said to do.
func TestAStopEndsTheRunAndTheRestIsSkipped(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL"}, {Kind: domain.ActionStop}, {Kind: "CREATE_BUCKET"},
	}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Errorf("status %q, want SUCCEEDED - stopping is not failing", run.Status)
	}
	if run.ActionResults[1].Status != domain.ActionSucceeded {
		t.Errorf("the STOP itself is %q", run.ActionResults[1].Status)
	}
	if run.ActionResults[2].Status != domain.ActionSkipped {
		t.Errorf("the action after the STOP is %q, want SKIPPED", run.ActionResults[2].Status)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d actions dispatched after a STOP", len(h.dispatcher.calls))
	}
}

// A STOP inside a branch ends the whole run, not just the arm it stands in.
func TestAStopInsideABranchEndsTheWholeRun(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		branchOf("true", []map[string]any{{"kind": string(domain.ActionStop)}}, nil),
		{Kind: "ADD_LABEL"},
	}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Errorf("status %q, want SUCCEEDED", run.Status)
	}
	last := run.ActionResults[len(run.ActionResults)-1]
	if last.Kind != "ADD_LABEL" || last.Status != domain.ActionSkipped {
		t.Errorf("the top-level action after the branch is %+v, want SKIPPED", last)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Errorf("%d actions dispatched after a nested STOP", len(h.dispatcher.calls))
	}
}

// G-07's key is (rule, occasion, action index), and a nested action has no index at the top level -
// two branches' first actions keyed by index would share a key and the second would silently do
// nothing. The key names the path.
func TestTwoBranchesFirstActionsDoNotShareAKey(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		branchOf("true", []map[string]any{{"kind": "ADD_LABEL"}}, nil),
		branchOf("true", []map[string]any{{"kind": "ADD_LABEL"}}, nil),
	}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(h.dispatcher.calls) != 2 {
		t.Fatalf("%d actions dispatched, want both branches' arms", len(h.dispatcher.calls))
	}

	keys := map[string]bool{}
	for _, result := range run.ActionResults {
		if result.Kind != "ADD_LABEL" {
			continue
		}
		if result.IdempotencyKey == "" {
			t.Fatalf("a nested action carries no key: %+v", result)
		}
		if keys[result.IdempotencyKey] {
			t.Fatalf("two branches' first actions share the key %q", result.IdempotencyKey)
		}
		keys[result.IdempotencyKey] = true
		if !strings.Contains(result.IdempotencyKey, result.Path) {
			t.Errorf("the key %q does not name the path %q", result.IdempotencyKey, result.Path)
		}
	}
	if len(keys) != 2 {
		t.Fatalf("%d nested results, want two", len(keys))
	}
}

// A branch whose condition cannot be evaluated fails the action rather than picking a default arm:
// a branch that quietly took `else` on a timeout would act out the opposite of what its rule says.
func TestABranchConditionThatCannotBeEvaluatedFailsTheAction(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		branchOf("nonsense", []map[string]any{{"kind": "ADD_LABEL"}}, nil),
		{Kind: "CREATE_BUCKET"},
	}
	h := newEngine(t, rule)

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunFailed {
		t.Errorf("status %q, want FAILED under on_error STOP", run.Status)
	}
	branch := run.ActionResults[0]
	if branch.Status != domain.ActionFailed || branch.ErrorCode == "" {
		t.Errorf("the branch's result is %+v, want FAILED with a code", branch)
	}
	if branch.Matched != nil {
		t.Errorf("a branch that decided nothing claims an answer: %+v", branch)
	}
	if run.ActionResults[1].Status != domain.ActionSkipped {
		t.Errorf("the action after the failed branch is %q, want SKIPPED", run.ActionResults[1].Status)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Errorf("%d actions dispatched under a branch that decided nothing", len(h.dispatcher.calls))
	}
}

// WAIT (G-09): the run suspends and a job resumes it. A WAIT of a day holds no worker, survives a
// restart - the moment lives on the job row - and resumes on time, proved with the fixed clock.

func waitingRule() domain.Rule {
	rule := enabledRule()
	// A real version, as every stored rule has one: the resume carries it, and the mid-wait edit
	// test bumps it.
	rule.Version = 1
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL"},
		{Kind: domain.ActionWait, Params: map[string]any{"duration": "P1D"}},
		{Kind: "CREATE_BUCKET"},
	}
	return rule
}

// resumeCommand is the command the queue would hand the engine when the parked job comes due,
// built from the request the suspension enqueued - which is the restart-survival argument: nothing
// but this payload and the run row is needed to continue.
func resumeCommand(t *testing.T, request queue.Request) Command {
	t.Helper()

	cmd := Command{Trigger: domain.TriggerKind(request.Payload["trigger"].(string))}
	ids := map[string]*shared.ID{
		"rule_id": &cmd.RuleID, "run_id": &cmd.RunID,
		"event_id": &cmd.EventID, "triggered_by": &cmd.TriggeredBy, "subject_id": &cmd.SubjectID,
	}
	for key, into := range ids {
		if text, present := request.Payload[key].(string); present {
			*into = shared.ID(text)
		}
	}
	cmd.Occasion, _ = request.Payload["occasion"].(string)
	cmd.ResumeFrom, _ = request.Payload["resume_from"].(string)
	cmd.RuleVersion, _ = request.Payload["rule_version"].(int)
	if depth, present := request.Payload["causation_depth"].(int); present {
		cmd.CausationDepth = depth
	}
	return cmd
}

// The acceptance criterion: a WAIT of a day parks the run - WAITING, not finished - and the queue
// holds the resume at exactly the moment the delay names.
func TestAWaitParksTheRunAndTheQueueCarriesTheResume(t *testing.T) {
	h := newEngine(t, waitingRule())

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunWaiting {
		t.Fatalf("status %q, want WAITING", run.Status)
	}
	if run.FinishedAt != nil {
		t.Error("a parked run claims to be finished")
	}
	// The results so far are written: the action before the WAIT, and nothing after it - what is
	// left is neither skipped nor recorded, because it is yet to run.
	if len(run.ActionResults) != 1 || run.ActionResults[0].Kind != "ADD_LABEL" {
		t.Errorf("the parked run records %+v, want just ADD_LABEL", run.ActionResults)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d actions dispatched before the WAIT", len(h.dispatcher.calls))
	}
	if stored := h.runs.last(); stored.Status != domain.RunWaiting {
		t.Errorf("the stored run says %q", stored.Status)
	}

	if len(h.jobs.requests) != 1 {
		t.Fatalf("%d jobs enqueued, want the one resume", len(h.jobs.requests))
	}
	request := h.jobs.requests[0]
	if request.Kind != queue.KindAutomationRun {
		t.Errorf("the resume is a %q job", request.Kind)
	}
	// Resumes on time: the queue's own run_at, a day out from the fixed clock. No worker sleeps.
	if want := now.Add(24 * time.Hour); !request.RunAt.Equal(want) {
		t.Errorf("the resume is due %v, want %v", request.RunAt, want)
	}
	if request.Payload["resume_from"] != "1" {
		t.Errorf("the resume points at %v, want the WAIT's path", request.Payload["resume_from"])
	}
	if request.Payload["run_id"] != run.ID.String() {
		t.Errorf("the resume names run %v", request.Payload["run_id"])
	}
	// No settlement while parked: the run is not over, so nothing announced a finish and nothing
	// touched the failure streak.
	for _, kind := range h.events.types() {
		if kind == event.RuleRunFinished || kind == event.RuleRunFailed {
			t.Errorf("a parked run announced %s", kind)
		}
	}
}

// The parked job comes due, the run finishes, and nothing before the WAIT acts twice - the
// restart-survival acceptance: the payload and the run row are all the resume needs.
func TestAResumedRunFinishesWithoutActingTwice(t *testing.T) {
	h := newEngine(t, waitingRule())

	parked, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("parking: %v", err)
	}

	resumed, err := h.engine.Execute(
		context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if resumed.Status != domain.RunSucceeded {
		t.Fatalf("status %q after the resume, want SUCCEEDED", resumed.Status)
	}
	if resumed.ID != parked.ID {
		t.Errorf("the resume produced a second run %s", resumed.ID)
	}
	if len(resumed.ActionResults) != 3 {
		t.Fatalf("the finished run records %+v", resumed.ActionResults)
	}
	if wait := resumed.ActionResults[1]; wait.Kind != domain.ActionWait ||
		wait.Status != domain.ActionSucceeded {
		t.Errorf("the WAIT's own result is %+v", wait)
	}

	// ADD_LABEL once before the WAIT and CREATE_BUCKET once after it: the replay appended the
	// recorded result rather than dispatching again.
	counts := map[string]int{}
	for _, call := range h.dispatcher.calls {
		counts[call.kind]++
	}
	if counts["ADD_LABEL"] != 1 || counts["CREATE_BUCKET"] != 1 {
		t.Errorf("dispatch counts %v, want each action exactly once", counts)
	}
}

// A redelivered resume finds the run finished and does nothing again.
func TestARedeliveredResumeDoesNothingTwice(t *testing.T) {
	h := newEngine(t, waitingRule())
	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("parking: %v", err)
	}

	cmd := resumeCommand(t, h.jobs.requests[0])
	for range 2 {
		if _, err := h.engine.Execute(context.Background(), engineActor(), cmd); err != nil {
			t.Fatalf("resuming: %v", err)
		}
	}
	if len(h.dispatcher.calls) != 2 {
		t.Errorf("%d dispatches over a delivered and a redelivered resume", len(h.dispatcher.calls))
	}
}

// A second WAIT further down parks the run again, on a resume of its own.
func TestASecondWaitParksTheRunAgain(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{
		{Kind: domain.ActionWait, Params: map[string]any{"duration": "PT1H"}},
		{Kind: "ADD_LABEL"},
		{Kind: domain.ActionWait, Params: map[string]any{"duration": "PT2H"}},
		{Kind: "CREATE_BUCKET"},
	}
	h := newEngine(t, rule)

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("parking: %v", err)
	}
	again, err := h.engine.Execute(
		context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if again.Status != domain.RunWaiting {
		t.Fatalf("status %q after the first resume, want WAITING on the second WAIT", again.Status)
	}
	if len(h.jobs.requests) != 2 {
		t.Fatalf("%d jobs enqueued, want one per suspension", len(h.jobs.requests))
	}
	if h.jobs.requests[1].Payload["resume_from"] != "2" {
		t.Errorf("the second resume points at %v", h.jobs.requests[1].Payload["resume_from"])
	}

	final, err := h.engine.Execute(
		context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[1]))
	if err != nil {
		t.Fatalf("finishing: %v", err)
	}
	if final.Status != domain.RunSucceeded {
		t.Errorf("status %q at the end, want SUCCEEDED", final.Status)
	}
	if len(final.ActionResults) != 4 {
		t.Errorf("the finished run records %+v", final.ActionResults)
	}
}

// A rule disabled while the run waited cannot keep acting. The run fails with a code naming what
// happened - and deliberately without touching the failure streak, because "somebody switched the
// rule off" is not the rule's actions failing.
func TestARuleDisabledMidWaitFailsTheRunWithoutTheStreak(t *testing.T) {
	h := newEngine(t, waitingRule())
	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("parking: %v", err)
	}

	disabled := waitingRule()
	disabled.Enabled = false
	h.rules.rows[ruleID] = disabled

	run, err := h.engine.Execute(
		context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if run.Status != domain.RunFailed || run.ErrorCode != "automation.rule_not_enabled" {
		t.Errorf("the orphaned run says %q / %q", run.Status, run.ErrorCode)
	}
	if h.failures.count != 0 {
		t.Errorf("the streak was bumped %d times for a disabled rule", h.failures.count)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d dispatches - a disabled rule's resume acted", len(h.dispatcher.calls))
	}
}

// A rule edited mid-wait is a different program: the recorded paths may no longer name its
// actions, so the resume refuses to run a mix of two rules.
func TestARuleEditedMidWaitDoesNotResume(t *testing.T) {
	h := newEngine(t, waitingRule())
	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("parking: %v", err)
	}

	edited := waitingRule()
	edited.Version++
	h.rules.rows[ruleID] = edited

	run, err := h.engine.Execute(
		context.Background(), engineActor(), resumeCommand(t, h.jobs.requests[0]))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if run.Status != domain.RunFailed || run.ErrorCode != "automation.rule_changed_while_waiting" {
		t.Errorf("the resumed run says %q / %q", run.Status, run.ErrorCode)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Errorf("%d dispatches - an edited rule's resume acted", len(h.dispatcher.calls))
	}
}

// SEND_WEBHOOK's event is not a value a rule can carry - it happens after the rule is written - so
// the run supplies it to the dispatcher beside the rule's own parameters (automation.md §2.2).
func TestTheRunSuppliesTheEventBesideTheRulesParameters(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{
		Kind:   "SEND_WEBHOOK",
		Params: map[string]any{"subscription_id": "01936f2a-7c1e-7000-8000-0000000000f7"},
	}}
	h := newEngine(t, rule)
	h.dispatcher.scopes["SEND_WEBHOOK"] = "automation:manage"

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}
	if len(h.dispatcher.calls) != 1 {
		t.Fatalf("%d dispatches", len(h.dispatcher.calls))
	}

	call := h.dispatcher.calls[0]
	if call.supplied["event_id"] != itemEvent().ID.String() {
		t.Errorf("the run supplied %v, want its event", call.supplied)
	}
	if _, carried := call.params["event_id"]; carried {
		t.Error("the event leaked into the rule's own parameters")
	}
}

// The JUMBLE_ENTRY runs (G-10): the conditions read the entry as data under `payload`, and the
// run supplies the entry to the actions a rule cannot carry it for.

type jumbleEntries struct{ entry jumbledomain.Entry }

func (j jumbleEntries) Find(_ context.Context, id shared.ID) (jumbledomain.Entry, error) {
	if j.entry.ID != id {
		return jumbledomain.Entry{}, shared.ErrNotFound.WithDetail("jumble.entry_not_found")
	}
	return j.entry, nil
}

func TestAJumbleRunReadsTheEntryAndSuppliesIt(t *testing.T) {
	entry := jumbledomain.Entry{
		ID:      shared.ID("01936f2a-7c1e-7000-8000-000000000ea1"),
		Channel: jumbledomain.ChannelWebhook, Sender: "orders@example.org",
		RawSubject: "Order #42", Status: jumbledomain.StatusNew,
	}

	rule := enabledRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerJumbleEntry}
	// `payload` resolves to the entry's fields: the fake compiler answers every declared name, so
	// what this proves is that the engine hands the run the jumble lookup and the entry's identity.
	rule.Conditions = []domain.Condition{{Expr: "payload.channel == 'WEBHOOK'"}}
	rule.Actions = []domain.Action{{Kind: "CONVERT_JUMBLE_ENTRY", Params: map[string]any{
		"collection_id": "01936f2a-7c1e-7000-8000-000000000ea2",
	}}}
	h := newEngine(t, rule)
	h.engine.Jumble = jumbleEntries{entry: entry}
	h.dispatcher.scopes["CONVERT_JUMBLE_ENTRY"] = "items:write"

	run, err := h.engine.Execute(context.Background(), engineActor(), Command{
		RuleID: ruleID, Trigger: domain.TriggerJumbleEntry,
		SubjectID: entry.ID, Occasion: entry.ID.String(),
	})
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("status %q: %+v", run.Status, run)
	}
	if run.SubjectID != entry.ID {
		t.Errorf("the run's subject is %s, want the entry", run.SubjectID)
	}

	if len(h.dispatcher.calls) != 1 {
		t.Fatalf("%d dispatches", len(h.dispatcher.calls))
	}
	call := h.dispatcher.calls[0]
	// The run supplies the entry: not a value a rule can carry, because the entry arrives after
	// the rule is written.
	if call.supplied["entry_id"] != entry.ID.String() {
		t.Errorf("the run supplied %v, want the entry", call.supplied)
	}
}

// tenantBudgetFake answers the verdict directly - the resolution is the quota engine's, tested
// there; this engine owes that an exhausted workspace budget throttles visibly (H-08).
type tenantBudgetFake struct {
	allowed bool
	asked   int
}

func (b *tenantBudgetFake) AutomationRuns(context.Context, string, time.Time) (bool, error) {
	b.asked++
	return b.allowed, nil
}

func TestTheTenantBudgetThrottlesTheWholeWorkspace(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	budget := &tenantBudgetFake{allowed: false}
	h.engine.Quota = budget

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if run.Status != domain.RunThrottled {
		t.Fatalf("status %q, want THROTTLED - the workspace's hour is spent", run.Status)
	}
	if len(h.dispatcher.calls) != 0 {
		t.Error("an over-budget run acted")
	}

	// Below the budget nothing changes.
	budget.allowed = true
	run, err = h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil || run.Status == domain.RunThrottled {
		t.Errorf("a permitted run throttled: (%q, %v)", run.Status, err)
	}
	if budget.asked != 2 {
		t.Errorf("the budget was asked %d times", budget.asked)
	}
}

// A rule-throttled run consumes nothing of the workspace budget: the rule's own bound answers
// first.
func TestARuleThrottleSparesTheTenantBudget(t *testing.T) {
	rule := enabledRule()
	rule.Throttle = domain.Throttle{MaxRunsPerHour: 10}
	h := newEngine(t, rule)
	h.runs.since = 11
	budget := &tenantBudgetFake{allowed: true}
	h.engine.Quota = budget

	if _, err := h.engine.Execute(context.Background(), engineActor(), command(0)); err != nil {
		t.Fatalf("running: %v", err)
	}
	if budget.asked != 0 {
		t.Error("a rule-throttled run reached the workspace budget")
	}
}
