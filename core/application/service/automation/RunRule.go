// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	outboxrepo "github.com/Jersyfi/hubtask/core/application/repository/outbox"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// MaxConsecutiveFailures is the run of failed runs after which a rule switches itself off
// (automation.md §2), with a notification to its owner.
//
// Five, and counted in runs rather than in actions. A rule whose third action is refused fails once,
// not once per action - the count is about the rule being wrong rather than about how much of it is.
// Five is enough that a transient outage does not disable a working rule and few enough that a rule
// pointing at something that no longer exists stops before anybody has to notice it.
const MaxConsecutiveFailures = 5

// Actions dispatches one of a rule's steps into the use case registry.
//
// Primitives rather than a shared struct, because the dispatcher lives in an adapter and the
// application layer may not name its types (ADR-0001). What crosses is a kind and a document, which
// is what the rule stored.
type Actions interface {
	Dispatch(
		ctx context.Context, runAs appshared.ActorContext, kind string, params map[string]any,
	) (usecase.Output, error)
}

// Scopes answers which token scope an action's use case declares.
//
// The engine grants the run exactly that scope and no other, per action. A rule presents no
// credential - it is not a token - so the scope bound, whose whole purpose is letting a *token* be
// narrower than its owner, has nothing to narrow. Granting the action's own scope is what makes the
// role the thing that decides, which is what "checked by the authoriser exactly as a person's would
// be" means (ADR-0005, automation.md §2). G-05 applied the credential bound where it belongs: to
// the person who wrote the rule.
type Scopes interface {
	ForAction(kind string) (string, bool)
}

// Owners is told when a rule switches itself off.
type Owners interface {
	RuleDisabled(ctx context.Context, rule domain.Rule, at time.Time) error
}

// RunRule is one rule's reaction to one event: the engine (G-07, automation.md §2).
//
// It runs inside the queue runner's transaction, which is what makes at-least-once delivery safe to
// build on. The run row, the effects its actions had, the idempotency records that make a redelivery
// harmless and the job's own completion all commit together - so a process that dies halfway leaves
// none of them and the job is claimed again.
//
// The engine gets no bypass. Every action goes through the same registry a person's request goes
// through, as the `run_as` account, and the authoriser answers it the way it answers anybody
// (rule 2). That is the whole point of `run_as`, and the reason G-05 spent its effort on who may
// write a rule at all.
type RunRule struct {
	Rules      repository.Rules
	Runs       repository.Runs
	Failures   repository.Failures
	Events     outboxrepo.Events
	Source     Source
	Dispatcher Actions
	Scopes     Scopes
	Conditions expression.Compiler
	Entries    Entries
	Containers Containers
	Guard      Idempotency
	Owners     Owners
	// Jobs is where a WAIT parks its resume (G-09). The engine runs inside the queue runner's
	// transaction, so the suspended run and the job that will resume it commit together - a
	// process that dies between them leaves neither.
	Jobs       Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Source reads the event a run is about, as it was written.
//
// Narrow rather than the outbox port, for the reason every slice here is: an engine that held the
// outbox could append to it, and the one event it may write goes through Events above with the
// depth incremented.
type Source interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// Idempotency is what makes a redelivered event harmless.
//
// The key is derived from the rule, the event and the action's index (automation.md §2), so the
// second delivery of one event finds every action already answered and does none of them again.
// The guard joins the runner's transaction rather than opening its own, which is right here: the
// reservation, the effect and the run commit together, so a run that died before committing left no
// reservation to block its retry.
type Idempotency interface {
	// Claim reserves the key and reports whether this attempt is the first. False means a previous
	// attempt already did the work.
	Claim(ctx context.Context, actor appshared.ActorContext, key string) (bool, error)
}

// RuleReader is the one read the queue adapter makes for itself: what a rule says about failure.
//
// Its own interface rather than the repository, because the adapter's question is not the engine's -
// the engine records what happened, and the layer above decides whether the job comes back.
type RuleReader interface {
	Find(ctx context.Context, id shared.ID) (domain.Rule, error)
}

// Command is one job's worth of work.
type Command struct {
	RuleID shared.ID
	// RunID is the identifier the run is to carry, when its producer already answered one to
	// somebody. `:trigger` does: it tells the caller what to watch for before the run exists, and a
	// run that then minted its own identifier would answer a different one. Zero everywhere else,
	// and the engine mints one.
	RunID shared.ID
	// Trigger is which of the rule's six ways of starting produced this job (G-08).
	//
	// It is checked against the rule rather than trusted: a job queued by the schedule pass and a
	// rule that has since been edited into an `EVENT` rule are a run that must not happen, and the
	// queue is where a job outlives the shape it was written for.
	Trigger domain.TriggerKind
	// EventID is the event that started the run, for the one kind an event starts.
	EventID shared.ID
	// TriggeredBy is who pulled it, for MANUAL, and SubjectID the entry a RELATIVE_DATE run is
	// about. Both zero for the kinds that have neither.
	TriggeredBy shared.ID
	SubjectID   shared.ID
	// Occasion is what makes this run's actions idempotent: the thing that happened once.
	//
	// The event for an `EVENT` run, the occurrence's instant for a schedule, the run itself for a
	// manual press or an inbound delivery. It has to be here rather than derived from the event,
	// because five of the six triggers have no event - and a key derived from a zero event
	// identifier would be the *same* key for every run of that rule for ever, so the second run
	// would find the first's answer stored and do nothing at all.
	Occasion string
	// Payload is the body an inbound delivery carried, as the CEL environment reads it. Empty for
	// every other kind.
	Payload        map[string]any
	CausationDepth int
	// ResumeFrom is the path of the WAIT a suspended run parked on, and empty for a fresh run.
	// The job the suspension enqueued carries it; the engine picks the run up at that action and
	// replays what the row already records instead of acting twice.
	ResumeFrom string
	// RuleVersion is the rule as the suspended run knew it. A rule edited mid-wait is a different
	// program - the recorded paths may no longer name its actions - so the resume refuses to run
	// a mix of two rules and fails the run with a code that says what happened.
	RuleVersion int
}

// occasion answers what makes this command's actions idempotent, reading the event when the caller
// named nothing. The fallback keeps every key an `EVENT` run has already written unchanged.
func (c Command) occasion() string {
	if c.Occasion != "" {
		return c.Occasion
	}
	return c.EventID.String()
}

// Execute runs one rule against one event and records what happened.
//
// The order is the order of what is cheapest to refuse. The depth is checked first because it needs
// nothing, then the throttle because it is one count, then the conditions because they are reads,
// and only then the actions because they are writes. A run that may not act must not evaluate
// conditions either - evaluating them is where the reads are, and a loop that read on every hop
// would cost exactly what the bound exists to prevent.
func (h RunRule) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd Command,
) (domain.Run, error) {
	now := h.Clock.Now()

	if cmd.ResumeFrom != "" {
		return h.resume(ctx, actor, cmd, now)
	}

	rule, err := h.Rules.Find(ctx, cmd.RuleID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// Deleted or disabled between the event and the run. Not a failure of anything: there
			// is no rule to record a run against, and a job for a rule nobody has is done.
			return domain.Run{}, nil
		}
		return domain.Run{}, err
	}
	if !rule.Enabled {
		return domain.Run{}, nil
	}
	if rule.Trigger.Kind != cmd.Trigger {
		// Edited into another kind between the job and the run. Not a failure of anything: the
		// producer that queued this no longer speaks for the rule, and a run recorded against a
		// trigger the rule no longer has would be a log entry that never happened.
		return domain.Run{}, nil
	}

	runID := cmd.RunID
	if runID.IsZero() {
		runID = h.IDs.NewID()
	}
	run, err := domain.StartRun(domain.NewRunInput{
		ID: runID, TenantID: actor.TenantID, RuleID: rule.ID, EventID: cmd.EventID,
		Trigger: cmd.Trigger, TriggeredBy: cmd.TriggeredBy, SubjectID: cmd.SubjectID,
		CausationDepth: cmd.CausationDepth, Now: now,
	})
	if err != nil {
		return domain.Run{}, err
	}
	if err := h.Runs.Start(ctx, run); err != nil {
		return domain.Run{}, err
	}
	h.announce(ctx, event.RuleRunStarted, rule, run, now)

	finished, err := h.decide(ctx, actor, rule, run, cmd, now)
	if err != nil {
		return domain.Run{}, err
	}
	if err := h.Runs.Finish(ctx, finished); err != nil {
		return domain.Run{}, err
	}
	if finished.Status == domain.RunWaiting {
		// Parked, not over: the suspension has already enqueued its own resume, and nothing is
		// settled - the failure streak and the finish event belong to the run's real end.
		return finished, nil
	}

	if err := h.settle(ctx, rule, finished, now); err != nil {
		return domain.Run{}, err
	}
	return finished, nil
}

// resume picks a suspended run up where its WAIT parked it (G-09).
//
// The run row is the memory: its recorded results are replayed without acting - a BRANCH descends
// the arm its recorded answer names, a performed action is not performed again - and live
// execution begins at the WAIT the resume names. The world may have moved while the run waited,
// and each possibility gets the honest answer: a run that is no longer WAITING was already
// resumed, so the redelivered job is done; a rule that is gone, disabled or edited cannot
// continue, and the run fails with a code naming which - deliberately without counting against
// the failure streak, because none of the three is the rule's actions failing.
func (h RunRule) resume(
	ctx context.Context, actor appshared.ActorContext, cmd Command, now time.Time,
) (domain.Run, error) {
	if cmd.RunID.IsZero() {
		return domain.Run{}, shared.ErrInternal.
			WithDetail("automation.run_payload_incomplete").
			WithParams(map[string]string{"field": "run_id"})
	}

	run, err := h.Runs.Find(ctx, cmd.RunID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// Swept by retention while the delay passed. There is nothing left to finish.
			return domain.Run{}, nil
		}
		return domain.Run{}, err
	}
	if run.Status != domain.RunWaiting {
		// Already resumed by an earlier delivery of this job. Done is done.
		return run, nil
	}

	rule, err := h.Rules.Find(ctx, cmd.RuleID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return h.orphan(ctx, domain.Rule{}, run, "automation.rule_not_found", now)
		}
		return domain.Run{}, err
	}
	if !rule.Enabled {
		return h.orphan(ctx, rule, run, "automation.rule_not_enabled", now)
	}
	if cmd.RuleVersion != 0 && rule.Version != cmd.RuleVersion {
		return h.orphan(ctx, rule, run, "automation.rule_changed_while_waiting", now)
	}

	envelope, err := h.envelope(ctx, cmd.EventID)
	if err != nil {
		return domain.Run{}, err
	}
	values := h.values(envelope, cmd, now)

	actions, halted, pending := h.act(ctx, actor, rule, cmd, values, replay{
		recorded: recordedByPath(run.ActionResults), resumeFrom: cmd.ResumeFrom,
	})

	var finished domain.Run
	switch {
	case pending != nil:
		// A second WAIT further down the rule. The run parks again, on a new resume of its own.
		if err := h.park(ctx, rule, run, cmd, pending, now); err != nil {
			return domain.Run{}, err
		}
		finished = run.Suspend(run.ConditionResults, actions)
	case halted:
		failed := run.Fail("automation.action_failed", now)
		failed.ConditionResults, failed.ActionResults = run.ConditionResults, actions
		finished = failed
	default:
		finished = run.Complete(run.ConditionResults, actions, now)
	}

	if err := h.Runs.Finish(ctx, finished); err != nil {
		return domain.Run{}, err
	}
	if finished.Status == domain.RunWaiting {
		return finished, nil
	}
	if err := h.settle(ctx, rule, finished, now); err != nil {
		return domain.Run{}, err
	}
	return finished, nil
}

// orphan ends a suspended run whose rule can no longer speak for it.
//
// Recorded and announced, so the log says why the run never finished its actions - but never
// settled: the streak that switches a rule off counts a rule's actions failing, and "somebody
// disabled the rule while it waited" is not that. The rule may be gone entirely, in which case
// the failure event has no rule to name an account from and is skipped rather than invented.
func (h RunRule) orphan(
	ctx context.Context, rule domain.Rule, run domain.Run, code string, now time.Time,
) (domain.Run, error) {
	failed := run.Fail(code, now)
	failed.ConditionResults, failed.ActionResults = run.ConditionResults, run.ActionResults
	if err := h.Runs.Finish(ctx, failed); err != nil {
		return domain.Run{}, err
	}
	if !rule.ID.IsZero() {
		h.announceFailure(ctx, rule, failed, false, now)
	}
	return failed, nil
}

// decide is everything between starting the run and writing how it ended.
func (h RunRule) decide(
	ctx context.Context, actor appshared.ActorContext,
	rule domain.Rule, run domain.Run, cmd Command, now time.Time,
) (domain.Run, error) {
	if domain.TooDeep(cmd.CausationDepth) {
		return run.Abort(now), nil
	}

	throttled, err := h.throttled(ctx, rule, now)
	if err != nil {
		return domain.Run{}, err
	}
	if throttled {
		return run.Throttle(now), nil
	}

	envelope, err := h.envelope(ctx, cmd.EventID)
	if err != nil {
		return domain.Run{}, err
	}
	values := h.values(envelope, cmd, now)

	conditions, matched, err := h.evaluate(ctx, rule, values)
	if err != nil {
		return domain.Run{}, err
	}
	if failure := firstConditionError(conditions); failure != "" {
		// A condition that could not be evaluated is not a condition that said no. The rule did not
		// decide anything, so the run failed rather than skipped - and the code says which
		// condition and why.
		failed := run.Fail(failure, now)
		failed.ConditionResults = conditions
		return failed, nil
	}
	if !matched {
		return run.Skip(conditions, now), nil
	}

	actions, stopped, pending := h.act(ctx, actor, rule, cmd, values, replay{})
	if pending != nil {
		if err := h.park(ctx, rule, run, cmd, pending, now); err != nil {
			return domain.Run{}, err
		}
		return run.Suspend(conditions, actions), nil
	}
	if stopped {
		failed := run.Fail("automation.action_failed", now)
		failed.ConditionResults, failed.ActionResults = conditions, actions
		return failed, nil
	}
	return run.Complete(conditions, actions, now), nil
}

// park enqueues the job that will resume a suspended run when its WAIT has passed (G-09).
//
// The queue's own run_at is the delay - no worker sleeps, and a restart changes nothing, because
// the moment lives on the job row rather than in a process. The job carries what the original
// command did plus the resume point: the queue is not a place to keep state the database has, and
// this is exactly what finding the run again needs. The rule's version travels with it, so a rule
// edited mid-wait is refused rather than resumed into a different program.
func (h RunRule) park(
	ctx context.Context, rule domain.Rule, run domain.Run, cmd Command,
	pending *suspension, now time.Time,
) error {
	if h.Jobs == nil {
		// Fail closed, and the job's retry brings the run back: a build with no queue wired
		// cannot promise the run ever resumes, and a run parked on that promise waits for ever.
		return shared.ErrInternal.WithDetail("automation.queue_unavailable")
	}

	payload := map[string]any{
		"rule_id":         rule.ID.String(),
		"trigger":         string(run.Trigger),
		"run_id":          run.ID.String(),
		"occasion":        cmd.occasion(),
		"resume_from":     pending.path,
		"rule_version":    rule.Version,
		"causation_depth": cmd.CausationDepth,
	}
	if !cmd.EventID.IsZero() {
		payload["event_id"] = cmd.EventID.String()
	}
	if !cmd.TriggeredBy.IsZero() {
		payload["triggered_by"] = cmd.TriggeredBy.String()
	}
	if !cmd.SubjectID.IsZero() {
		payload["subject_id"] = cmd.SubjectID.String()
	}
	if len(cmd.Payload) > 0 {
		payload["payload"] = cmd.Payload
	}

	_, err := h.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindAutomationRun,
		TenantID: run.TenantID,
		Payload:  payload,
		// Unique per suspension, so a redelivered job that parks the run again collapses into
		// the resume that is already scheduled rather than scheduling a second one.
		DedupeKey: ConsumerName + ":resume:" + run.ID.String() + ":" + pending.path,
		RunAt:     now.Add(pending.delay),
	})
	return err
}

// values is what the run's expressions are told, built once for the whole run.
//
// The command's subject and payload are on it beside the envelope, which is what makes a condition
// written for one trigger readable under another: `item` is the entry a RELATIVE_DATE run measures
// from exactly as it is the entry an event was about, and `payload` is the delivery's body or an
// empty document.
func (h RunRule) values(envelope event.Envelope, cmd Command, now time.Time) eventValues {
	return eventValues{
		envelope: envelope, now: now,
		subject: cmd.SubjectID, payload: cmd.Payload,
		entries: h.Entries, containers: h.Containers,
	}
}

// throttled answers whether the rule has already run as often as it may.
func (h RunRule) throttled(ctx context.Context, rule domain.Rule, now time.Time) (bool, error) {
	if rule.Throttle.MaxRunsPerHour <= 0 {
		return false, nil
	}

	count, err := h.Runs.CountSince(ctx, rule.ID, now.Add(-time.Hour))
	if err != nil {
		return false, err
	}
	// The run being decided is already in the log - it was written before this - so the bound is
	// reached when the count *exceeds* it rather than equals it. Off by one in the other direction
	// would let a rule bounded at one never run at all.
	return count > rule.Throttle.MaxRunsPerHour, nil
}

// envelope reads the event the run is about, or an empty one for a run nothing published started.
func (h RunRule) envelope(ctx context.Context, id shared.ID) (event.Envelope, error) {
	if id.IsZero() || h.Source == nil {
		return event.Envelope{}, nil
	}

	envelope, err := h.Source.FindEvent(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		// Swept by retention while the job waited. The conditions then see an empty event, which is
		// honest: what the rule would have decided about it is no longer knowable.
		return event.Envelope{}, nil
	}
	return envelope, err
}

// evaluate answers every condition, and whether they all held.
//
// Every one, not up to the first false. A run log that stopped at the first no would answer "why did
// this not happen" with one line where somebody wants the whole picture - and the cost is bounded by
// MaxConditions with a timeout each.
func (h RunRule) evaluate(
	ctx context.Context, rule domain.Rule, values eventValues,
) ([]domain.ConditionResult, bool, error) {
	results := make([]domain.ConditionResult, 0, len(rule.Conditions))
	matched := true

	if len(rule.Conditions) == 0 {
		return results, true, nil
	}
	if h.Conditions == nil {
		return nil, false, shared.ErrInternal.WithDetail("automation.expression_engine_unavailable")
	}

	for i, each := range rule.Conditions {
		result := domain.ConditionResult{Index: i}

		program, err := h.Conditions.Compile(each.Expr, condition.RuleEnvironment(), expression.Boolean)
		if err != nil {
			result.ErrorCode = codeOf(err)
			results, matched = append(results, result), false
			continue
		}
		out, err := program.Evaluate(ctx, values)
		if err != nil {
			result.ErrorCode = codeOf(err)
			results, matched = append(results, result), false
			continue
		}
		result.Matched = out.Bool
		if !out.Bool {
			matched = false
		}
		results = append(results, result)
	}
	return results, matched, nil
}

// act walks the rule's action tree and reports whether the run was stopped by a failure.
//
// `on_error` decides what a failure does to the rest, and the three values do what they say. STOP
// ends the run and the actions after it are SKIPPED - which is not the same as an action that ran
// and did nothing. CONTINUE runs the rest and the run still succeeds, because the run did what its
// rule says. RETRY is not decided here: it hands the job back to the queue, whose backoff and dead
// letter are what "retry" means in this system, and the handler above translates it.
//
// A tree rather than a list since G-09: a BRANCH carries two arms and the run takes the one its
// condition says, a STOP ends the run deliberately, and every result names its path. The arm a
// branch did not take is recorded nowhere - it was never part of this run, and the rule itself is
// where a reader sees what would have been there.
func (h RunRule) act(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, cmd Command,
	values eventValues, prior replay,
) ([]domain.ActionResult, bool, *suspension) {
	w := &walk{
		engine: h, actor: actor, rule: rule, occasion: cmd.occasion(), values: values,
		replay: prior,
	}
	w.list(ctx, rule.Actions, "")
	return w.results, w.halted, w.pending
}

// replay is what a resumed run already knows: the results its row recorded, and the WAIT it
// parked on. Empty for a fresh run.
type replay struct {
	recorded   map[string]domain.ActionResult
	resumeFrom string
}

// recordedByPath indexes a suspended run's results for the replay.
func recordedByPath(results []domain.ActionResult) map[string]domain.ActionResult {
	recorded := make(map[string]domain.ActionResult, len(results))
	for _, result := range results {
		recorded[result.Path] = result
	}
	return recorded
}

// suspension is a WAIT the walk reached: where the run parks, and for how long.
type suspension struct {
	path  string
	delay time.Duration
}

// walk is one run's pass over its rule's action tree.
type walk struct {
	engine   RunRule
	actor    appshared.ActorContext
	rule     domain.Rule
	occasion string
	values   eventValues
	replay   replay
	results  []domain.ActionResult
	// pending is a WAIT that parks the run: the walk stops where it stands, and what is left is
	// neither skipped nor recorded - it is yet to run, when the resume comes back for it.
	pending *suspension
	// halted is a failure under `on_error: STOP` - the run will fail. ended is a STOP action - the
	// run succeeded, because stopping early is what the rule said to do. Both skip what is left.
	halted bool
	ended  bool
}

// list performs one list of actions - the rule's own, or a branch's arm - under a parent path.
func (w *walk) list(ctx context.Context, actions []domain.Action, parent string) {
	if w.results == nil {
		w.results = make([]domain.ActionResult, 0, len(actions))
	}

	for i, action := range actions {
		if w.pending != nil {
			return
		}
		path := domain.ActionPath(parent, i)
		result := domain.ActionResult{Index: i, Kind: action.Kind, Path: path}
		if w.halted || w.ended {
			// Skipped without descending into a branch: no arm was chosen, so neither arm's
			// actions were ever this run's to reach.
			result.Status = domain.ActionSkipped
			w.results = append(w.results, result)
			continue
		}
		if prior, done := w.replay.recorded[path]; done {
			// A resumed run replays what its row recorded rather than acting twice - and a
			// recorded BRANCH descends the arm its recorded answer names, without re-evaluating a
			// condition whose world has moved while the run waited.
			w.results = append(w.results, prior)
			w.descendRecorded(ctx, action, path, prior)
			continue
		}

		switch action.Kind {
		case domain.ActionStop:
			// The run ends where it stands, and it succeeded: stopping early is what the rule
			// said to do.
			result.Status = domain.ActionSucceeded
			w.results = append(w.results, result)
			w.ended = true
		case domain.ActionBranch:
			w.branch(ctx, action, path, result)
		case domain.ActionWait:
			w.wait(action, path, result)
		default:
			result.IdempotencyKey = idempotencyKey(w.rule.ID, w.occasion, path)
			w.record(result,
				w.engine.dispatch(ctx, w.actor, w.rule, action, result.IdempotencyKey))
		}
	}
}

// descendRecorded follows a replayed BRANCH into the arm its recorded answer names.
func (w *walk) descendRecorded(
	ctx context.Context, action domain.Action, path string, prior domain.ActionResult,
) {
	if action.Kind != domain.ActionBranch || prior.Matched == nil {
		return
	}
	branch, err := domain.ReadBranch(action.Params, path, 0)
	if err != nil {
		// Unreachable: the resume checked the rule's version, so the parameters are the ones the
		// recorded walk read.
		return
	}
	arm, name := branch.Then, "then"
	if !*prior.Matched {
		arm, name = branch.Else, "else"
	}
	w.list(ctx, arm, path+"/"+name)
}

// wait parks the run - or lets it pass the one WAIT the resume names as already elapsed.
func (w *walk) wait(action domain.Action, path string, result domain.ActionResult) {
	if path == w.replay.resumeFrom {
		// The delay this resume is about has passed: the queue's own run_at is what measured it,
		// which is what "resumes on time" means with no worker held.
		result.Status = domain.ActionSucceeded
		w.results = append(w.results, result)
		return
	}

	delay, err := domain.WaitFor(action.Params, path)
	if err != nil {
		// Unreachable through the aggregate, which read the same parameters - but a delay this
		// walk cannot read is a run it cannot promise to resume, so it fails rather than guesses.
		w.record(result, err)
		return
	}
	// The WAIT itself is not recorded yet: it succeeds when the run resumes, and until then the
	// parked run's last written result is honestly the action before it.
	w.pending = &suspension{path: path, delay: delay}
}

// branch evaluates a BRANCH's condition and takes the arm it says.
//
// The condition is evaluated with the run's own values - the same environment, the same single
// `now` - and its answer is recorded on the result, which is what keeps an empty arm readable. A
// condition that cannot be evaluated fails the action rather than picking a default arm: a branch
// that quietly took `else` on a timeout would act out the opposite of what its rule says.
func (w *walk) branch(
	ctx context.Context, action domain.Action, path string, result domain.ActionResult,
) {
	branch, err := domain.ReadBranch(action.Params, path, 0)
	if err != nil {
		// Unreachable through the aggregate, which read the same parameters when the rule was
		// written. Failed rather than swallowed, because a branch the engine cannot read is a
		// branch whose arm it cannot choose.
		w.record(result, err)
		return
	}

	if w.engine.Conditions == nil {
		w.record(result, shared.ErrInternal.WithDetail("automation.expression_engine_unavailable"))
		return
	}
	program, err := w.engine.Conditions.Compile(
		branch.Condition, condition.RuleEnvironment(), expression.Boolean)
	if err != nil {
		w.record(result, err)
		return
	}
	out, err := program.Evaluate(ctx, w.values)
	if err != nil {
		w.record(result, err)
		return
	}

	matched := out.Bool
	result.Status, result.Matched = domain.ActionSucceeded, &matched
	w.results = append(w.results, result)

	arm, name := branch.Then, "then"
	if !matched {
		arm, name = branch.Else, "else"
	}
	w.list(ctx, arm, path+"/"+name)
}

// record writes one result, applying `on_error` to a failure.
func (w *walk) record(result domain.ActionResult, err error) {
	if err != nil {
		result.Status, result.ErrorCode = domain.ActionFailed, codeOf(err)
		if w.rule.OnError == domain.OnErrorStop {
			w.halted = true
		}
	} else {
		result.Status = domain.ActionSucceeded
	}
	w.results = append(w.results, result)
}

// dispatch performs one action as the rule's account.
func (h RunRule) dispatch(
	ctx context.Context, actor appshared.ActorContext,
	rule domain.Rule, action domain.Action, key string,
) error {
	if h.Guard != nil {
		first, err := h.Guard.Claim(ctx, actor, key)
		if err != nil {
			return err
		}
		if !first {
			// A previous attempt already did this. Succeeding rather than repeating is the whole
			// of at-least-once delivery being safe: a redelivered event re-runs into the stored
			// answer instead of acting twice.
			return nil
		}
	}

	runAs, err := h.runAs(actor, rule, action)
	if err != nil {
		return err
	}
	_, err = h.Dispatcher.Dispatch(ctx, runAs, action.Kind, action.Params)
	return err
}

// runAs builds the actor one action is performed as.
//
// The rule's account, and exactly the scope that action's use case declares. See Scopes for why a
// rule is granted the scope rather than narrowed by one.
//
// The name is deliberately empty. It is the label the audit trail denormalises, and a service
// account's is read where the trail is written rather than carried here - a rule that cached one
// would record a name that is a release out of date.
func (h RunRule) runAs(
	actor appshared.ActorContext, rule domain.Rule, action domain.Action,
) (appshared.ActorContext, error) {
	scope, known := "", false
	if h.Scopes != nil {
		scope, known = h.Scopes.ForAction(action.Kind)
	}
	if !known {
		return appshared.ActorContext{}, shared.ErrValidation.
			WithDetail("automation.action_unknown").
			WithParams(map[string]string{"action": action.Kind})
	}

	runAs := appshared.ActorContext{
		Kind:      appshared.ActorServiceAccount,
		TenantID:  actor.TenantID,
		AccountID: rule.RunAs,
		Locale:    actor.Locale,
		TimeZone:  actor.TimeZone,
	}
	if scope != "" {
		runAs.Scopes = []string{scope}
	}
	return runAs, nil
}

// settle counts the run against the rule and switches the rule off if it has failed often enough.
func (h RunRule) settle(
	ctx context.Context, rule domain.Rule, run domain.Run, now time.Time,
) error {
	if run.Status != domain.RunFailed {
		// Anything that is not a failure ends the streak - including a skip and a throttle. A rule
		// whose conditions said no is a rule that is working, and counting that as progress towards
		// being switched off would disable the most careful rules first.
		h.announce(ctx, event.RuleRunFinished, rule, run, now)
		return h.Failures.Clear(ctx, rule.ID, now)
	}

	count, err := h.Failures.Bump(ctx, rule.ID, now)
	if err != nil {
		return err
	}

	disabled := false
	if count >= MaxConsecutiveFailures {
		if disabled, err = h.Failures.Disable(ctx, rule.ID, MaxConsecutiveFailures, now); err != nil {
			return err
		}
	}
	h.announceFailure(ctx, rule, run, disabled, now)

	if disabled && h.Owners != nil {
		// The owner is told through the path C-09 built rather than a new channel: a rule that
		// switched itself off is exactly the kind of thing somebody has to be told about, and a
		// notification nobody receives is the same as no notification.
		return h.Owners.RuleDisabled(ctx, rule, now)
	}
	return nil
}

// announce publishes one of the two run events that are not failures.
//
// Best effort in the sense that it fails the transaction rather than being skipped: the event is
// written into the same transaction as the run, so a run that is recorded and an event that is not
// cannot come apart (ADR-0007).
func (h RunRule) announce(
	ctx context.Context, kind event.Type, rule domain.Rule, run domain.Run, now time.Time,
) {
	if h.Events == nil {
		return
	}

	payload := map[string]any{
		"run_id": run.ID.String(), "rule_id": rule.ID.String(),
		"causation_depth": run.CausationDepth,
	}
	if !run.EventID.IsZero() {
		payload["event_id"] = run.EventID.String()
	}
	if kind == event.RuleRunFinished {
		payload["status"] = string(run.Status)
		payload["actions_succeeded"] = run.Succeeded()
		payload["actions_failed"] = run.Failed()
	}
	h.publish(ctx, kind, rule, run, payload, now)
}

func (h RunRule) announceFailure(
	ctx context.Context, rule domain.Rule, run domain.Run, disabled bool, now time.Time,
) {
	if h.Events == nil {
		return
	}

	payload := map[string]any{
		"run_id": run.ID.String(), "rule_id": rule.ID.String(),
		"causation_depth": run.CausationDepth,
		"status":          string(run.Status), "rule_disabled": disabled,
	}
	if !run.EventID.IsZero() {
		payload["event_id"] = run.EventID.String()
	}
	if run.ErrorCode != "" {
		payload["error_code"] = run.ErrorCode
	}
	h.publish(ctx, event.RuleRunFailed, rule, run, payload, now)
}

// publish writes one run event.
//
// The depth is the run's plus one, which is what makes the chain countable: an event this run caused
// is one hop further from the act a person performed, and a rule that reacted to it would be at that
// depth. Nothing reacts to these today - the subscriber refuses its own three types - and the depth
// is still right, because the value is a fact about the event rather than a prediction about who
// reads it.
func (h RunRule) publish(
	ctx context.Context, kind event.Type, rule domain.Rule, run domain.Run,
	payload map[string]any, now time.Time,
) {
	envelope := event.Envelope{
		ID:       h.IDs.NewID(),
		Type:     kind,
		TenantID: run.TenantID,
		Subject:  "rule_run/" + run.ID.String(),
		Actor: event.Actor{
			Kind: shared.ActorServiceAccount, ID: rule.RunAs,
		},
		OccurredAt:     now.UTC(),
		CausationID:    run.EventID,
		CausationDepth: run.CausationDepth + 1,
		Payload:        payload,
	}
	if envelope.TenantID.IsZero() {
		envelope.TenantID = rule.TenantID
	}
	// The error is deliberately dropped rather than failing the run. An event nobody could write is
	// worth less than a run nobody recorded, and Append fails the transaction on a real database
	// error anyway - what is swallowed here is the case where no outbox is wired at all.
	_ = h.Events.Append(ctx, envelope)
}

// idempotencyKey is what automation.md §2 specifies: the rule, the occasion and the action's place
// in it.
//
// The place rather than the action's kind, because a rule may name one kind twice - "add this label
// and that one" is two actions of one kind, and a key that collapsed them would perform the first
// and silently skip the second. Since G-09 the place is a path rather than an index: a nested
// action has no index at the top level, and two branches' first actions keyed by index would share
// a key and the second would silently do nothing. A top-level action's path *is* its index, so
// every key an earlier release wrote is unchanged.
//
// The occasion is the event for an `EVENT` run and the thing that happened once for each of the
// other five (Command.Occasion). §2 writes "event_id" because when it was written that was the only
// way a run could start; what the sentence means is "the one occurrence this run answers", and a
// schedule's occurrence or a person's press is that occurrence exactly as an event is.
func idempotencyKey(ruleID shared.ID, occasion string, path string) string {
	return "automation:" + ruleID.String() + ":" + occasion + ":" + path
}

// firstConditionError answers the code of the first condition that could not be evaluated, and the
// empty string when every one of them answered.
func firstConditionError(results []domain.ConditionResult) string {
	for _, result := range results {
		if result.ErrorCode != "" {
			return result.ErrorCode
		}
	}
	return ""
}

// codeOf reads the message code out of a refusal, which is what the run records: the use case's own
// answer, unchanged, rather than a sentence this engine wrote.
func codeOf(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) && coded.DetailCode != "" {
		return coded.DetailCode
	}
	return "automation.action_failed"
}
