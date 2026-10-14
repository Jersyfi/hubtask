// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AiProviderResolver is what answers which provider a workspace uses. The adapter satisfies
// it; this file wraps it.
type AiProviderResolver interface {
	For(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, error)
}

// SourcedResolver answers the provider and which source it is - the adapter's Resolver.
type SourcedResolver interface {
	Sourced(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, aiprovider.Source, error)
}

// AiBudget is the slice of the quota guard this needs (multi-tenancy.md §4): one budget per
// source, the offered model's and the workspace's own (ADR-0072 §4).
type AiBudget interface {
	// AiTokens reports whether the source's daily budget still has room.
	AiTokens(ctx context.Context, tenant string, own bool, now time.Time) (bool, error)
	// MeterAi records what a call to the source cost, after the provider answered.
	MeterAi(ctx context.Context, own bool, at time.Time, tokens int64) error
	// AiRenewal is when the day that `now` falls on ends for the workspace: the next midnight in
	// its time zone, when a spent budget has room again.
	AiRenewal(ctx context.Context, now time.Time) (time.Time, error)
}

// Budgeted is the provider resolver with `ai-first.md` §2's per-tenant budget counter around it.
//
// **In the application layer rather than in the adapter**, which ADR-0049 decision 1 asked for and
// this is the first thing that needed it: the budget is a quota, a quota is read from the
// workspace's settings inside a transaction, and an outbound adapter that opened one would be an
// adapter driving the application layer (project-structure.md §2).
//
// What it produces when the budget is spent is a provider that **refuses like an absent one**: zero
// capabilities, and `ErrUnavailable` from both calls. That is the whole of the degradation, and it
// needed no new handling anywhere, because every caller already deals with a provider that cannot
// do something - the embedding pass finds nothing to do, the search runs lexically, and a
// suggestion is not made. A budget with a refusal shape of its own would have been a second way for
// each of them to degrade (ADR-0049 decision: one refusal, `ai.unavailable`). It carries the reason
// and the moment the budget renews as parameters, because the person reading it can do something an
// absent provider does not let them: wait until then.
type Budgeted struct {
	Providers  SourcedResolver
	Budget     AiBudget
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

var _ AiProviderResolver = Budgeted{}

// For answers the workspace's provider, or one that refuses because the budget is spent.
func (b Budgeted) For(
	ctx context.Context, actor appshared.ActorContext,
) (aiprovider.Provider, error) {
	provider, source, err := b.Providers.Sourced(ctx, actor)
	if err != nil || b.Budget == nil || source == aiprovider.SourceNone {
		// Nothing to call is nothing to count: a refusal keeps its own answer.
		return provider, err
	}
	own := source == aiprovider.SourceOwn

	var room bool
	var renews time.Time
	if err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			now := b.Clock.Now()
			var err error
			if room, err = b.Budget.AiTokens(ctx, actor.TenantID.String(), own, now); err != nil || room {
				return err
			}
			renews, err = b.Budget.AiRenewal(ctx, now)
			return err
		}); err != nil {
		return nil, err
	}
	if !room {
		return exhausted{renews: renews}, nil
	}
	return metered{provider: provider, budget: b.Budget, uow: b.UnitOfWork,
		scope: actor.PersistenceScope(), clock: b.Clock, own: own}, nil
}

// exhausted is a workspace that has spent its day's budget.
//
// It has what an installation with no provider has, which is the point: "over budget" and "not
// configured" are the same fact to everything downstream - there is no model to ask - and nine
// callers need no second handling. Only its refusal differs, and only in what a person reads: the
// budget, and when it renews (aiprovider.BudgetSpent).
type exhausted struct{ renews time.Time }

var _ aiprovider.Refusing = exhausted{}

func (exhausted) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "noop"}
}

// Refusal is what every call answers, asked before any is made.
func (e exhausted) Refusal() error { return aiprovider.BudgetSpent(e.renews) }

func (e exhausted) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, e.Refusal()
}

func (e exhausted) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, e.Refusal()
}

// metered is the workspace's real provider with the ledger written after each answer.
//
// **After, not before**, and that is the shape of the bound rather than a shortcut: nobody knows
// what a call will cost until the provider says so, so a budget cannot be reserved. The ceiling is
// checked against what has been spent, and the call that crosses it is the last one rather than one
// that never happened - which is the right way round for a bound whose purpose is to stop a runaway
// loop rather than to bill exactly.
//
// A call that failed meters nothing: a provider that refused charged nothing, and a budget counting
// refusals would tighten on itself the way the automation bound would if it counted its throttles.
type metered struct {
	provider aiprovider.Provider
	budget   AiBudget
	uow      persistence.UnitOfWork
	scope    persistence.Scope
	clock    clock.Clock
	// own is which budget the call counts against: the workspace's own model's, or the offered
	// one's.
	own bool
}

func (m metered) Capabilities() aiprovider.ProviderCapabilities {
	return m.provider.Capabilities()
}

// MeasureEmbedding forwards to the provider where it can measure, and answers zero where it
// cannot - a description is not metered, because no budget counts a question about a model.
func (m metered) MeasureEmbedding(ctx context.Context) (int, error) {
	if measured, can := m.provider.(aiprovider.Measured); can {
		return measured.MeasureEmbedding(ctx)
	}
	return 0, nil
}

var _ aiprovider.Measured = metered{}

func (m metered) Complete(
	ctx context.Context, request aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	answer, err := m.provider.Complete(ctx, request)
	if err != nil {
		return answer, err
	}
	return answer, m.meter(ctx, answer.Usage)
}

func (m metered) Embed(
	ctx context.Context, texts []string,
) (aiprovider.EmbeddingResult, error) {
	answer, err := m.provider.Embed(ctx, texts)
	if err != nil {
		return answer, err
	}
	return answer, m.meter(ctx, answer.Usage)
}

// meter writes the tally in a transaction of its own, because the provider call it follows happened
// outside one - which is where every provider call in this product happens
// (observability-reliability.md §8).
func (m metered) meter(ctx context.Context, usage aiprovider.Usage) error {
	tokens := int64(usage.InputTokens + usage.OutputTokens)
	if tokens <= 0 {
		// A provider that reports no usage - Ollama does not always - costs nothing this can
		// count. The budget then bounds nothing for that provider, which is honest: it is a local
		// model, and what it spends is the operator's own electricity.
		return nil
	}
	return m.uow.Within(ctx, m.scope, func(ctx context.Context) error {
		return m.budget.MeterAi(ctx, m.own, m.clock.Now(), tokens)
	})
}
