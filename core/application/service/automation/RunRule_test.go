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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
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
	kind   string
	actor  appshared.ActorContext
	params map[string]any
}

func newDispatched() *dispatched {
	return &dispatched{
		refuse: map[string]error{},
		scopes: map[string]string{"ADD_LABEL": "items:write", "CREATE_BUCKET": "containers:write"},
	}
}

func (d *dispatched) Dispatch(
	_ context.Context, runAs appshared.ActorContext, kind string, params map[string]any,
) (usecase.Output, error) {
	d.calls = append(d.calls, dispatchCall{kind: kind, actor: runAs, params: params})
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

type engineHarness struct {
	engine     RunRule
	rules      *ruleStore
	runs       *runLog
	failures   *failures
	dispatcher *dispatched
	events     *published
	owners     *told
	claims     *claims
}

func newEngine(t *testing.T, rule domain.Rule) *engineHarness {
	t.Helper()

	store := newRuleStore(rule)
	h := &engineHarness{
		rules: store, runs: newRunLog(), failures: &failures{},
		dispatcher: newDispatched(), events: &published{}, owners: &told{}, claims: newClaims(),
	}
	h.engine = RunRule{
		Rules: store, Runs: h.runs, Failures: h.failures, Events: h.events,
		Source:     source{envelope: itemEvent()},
		Dispatcher: h.dispatcher, Scopes: h.dispatcher,
		Conditions: compiler{}, Guard: h.claims, Owners: h.owners,
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
	return Command{RuleID: ruleID, EventID: itemEvent().ID, CausationDepth: depth}
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
