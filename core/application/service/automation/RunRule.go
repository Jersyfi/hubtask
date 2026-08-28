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
	RuleID         shared.ID
	EventID        shared.ID
	CausationDepth int
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

	run, err := domain.StartRun(domain.NewRunInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID, RuleID: rule.ID, EventID: cmd.EventID,
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

	if err := h.settle(ctx, rule, finished, now); err != nil {
		return domain.Run{}, err
	}
	return finished, nil
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

	conditions, matched, err := h.evaluate(ctx, rule, envelope, now)
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

	actions, stopped := h.act(ctx, actor, rule, run, envelope)
	if stopped {
		failed := run.Fail("automation.action_failed", now)
		failed.ConditionResults, failed.ActionResults = conditions, actions
		return failed, nil
	}
	return run.Complete(conditions, actions, now), nil
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
	ctx context.Context, rule domain.Rule, envelope event.Envelope, now time.Time,
) ([]domain.ConditionResult, bool, error) {
	results := make([]domain.ConditionResult, 0, len(rule.Conditions))
	matched := true

	if len(rule.Conditions) == 0 {
		return results, true, nil
	}
	if h.Conditions == nil {
		return nil, false, shared.ErrInternal.WithDetail("automation.expression_engine_unavailable")
	}

	values := eventValues{
		envelope: envelope, now: now, entries: h.Entries, containers: h.Containers,
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

// act dispatches the rule's steps and reports whether the run was stopped by a failure.
//
// `on_error` decides what a failure does to the rest, and the three values do what they say. STOP
// ends the run and the actions after it are SKIPPED - which is not the same as an action that ran
// and did nothing. CONTINUE runs the rest and the run still succeeds, because the run did what its
// rule says. RETRY is not decided here: it hands the job back to the queue, whose backoff and dead
// letter are what "retry" means in this system, and the handler above translates it.
func (h RunRule) act(
	ctx context.Context, actor appshared.ActorContext,
	rule domain.Rule, run domain.Run, envelope event.Envelope,
) ([]domain.ActionResult, bool) {
	results := make([]domain.ActionResult, 0, len(rule.Actions))
	stopped := false

	for i, action := range rule.Actions {
		result := domain.ActionResult{Index: i, Kind: action.Kind}
		if stopped {
			result.Status = domain.ActionSkipped
			results = append(results, result)
			continue
		}

		result.IdempotencyKey = idempotencyKey(rule.ID, run.EventID, i)
		if err := h.dispatch(ctx, actor, rule, action, result.IdempotencyKey); err != nil {
			result.Status, result.ErrorCode = domain.ActionFailed, codeOf(err)
			if rule.OnError == domain.OnErrorStop {
				stopped = true
			}
		} else {
			result.Status = domain.ActionSucceeded
		}
		results = append(results, result)
	}
	return results, stopped
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

// idempotencyKey is what automation.md §2 specifies: the rule, the event and the action's index.
//
// The index rather than the action's kind, because a rule may name one kind twice - "add this label
// and that one" is two actions of one kind, and a key that collapsed them would perform the first
// and silently skip the second.
func idempotencyKey(ruleID, eventID shared.ID, index int) string {
	return "automation:" + ruleID.String() + ":" + eventID.String() + ":" + itoa(index)
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
