// SPDX-License-Identifier: BUSL-1.1
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

// AiProviderResolver is what answers which provider a workspace uses (J-03). The adapter satisfies
// it; this file wraps it.
type AiProviderResolver interface {
	For(ctx context.Context, actor appshared.ActorContext) (aiprovider.Provider, error)
}

// AiBudget is the slice of the quota guard this needs (J-15, H-08's machinery).
type AiBudget interface {
	// AiTokens reports whether the workspace's daily budget still has room.
	AiTokens(ctx context.Context, tenant string, now time.Time) (bool, error)
	// MeterAi records what a call cost, after the provider answered.
	MeterAi(ctx context.Context, at time.Time, tokens int64) error
}

// Budgeted is the provider resolver with `ai-first.md` §2's per-tenant budget counter around it
// (J-15).
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
// each of them to degrade (ADR-0049 decision: one refusal, `ai.unavailable`).
type Budgeted struct {
	Providers  AiProviderResolver
	Budget     AiBudget
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

var _ AiProviderResolver = Budgeted{}

// For answers the workspace's provider, or one that refuses because the budget is spent.
func (b Budgeted) For(
	ctx context.Context, actor appshared.ActorContext,
) (aiprovider.Provider, error) {
	provider, err := b.Providers.For(ctx, actor)
	if err != nil || b.Budget == nil {
		return provider, err
	}

	var room bool
	if err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			var err error
			room, err = b.Budget.AiTokens(ctx, actor.TenantID.String(), b.Clock.Now())
			return err
		}); err != nil {
		return nil, err
	}
	if !room {
		return exhausted{}, nil
	}
	return metered{provider: provider, budget: b.Budget, uow: b.UnitOfWork,
		scope: actor.PersistenceScope(), clock: b.Clock}, nil
}

// exhausted is a workspace that has spent its day's budget.
//
// It answers exactly what an installation with no provider answers, which is the point: "over
// budget" and "not configured" are the same fact to everything downstream - there is no model to
// ask - and the difference between them belongs in the operator's quota view, not in nine callers'
// error handling.
type exhausted struct{}

func (exhausted) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{Kind: "noop"}
}

func (exhausted) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	return aiprovider.CompletionResult{}, aiprovider.ErrUnavailable
}

func (exhausted) Embed(context.Context, []string) (aiprovider.EmbeddingResult, error) {
	return aiprovider.EmbeddingResult{}, aiprovider.ErrUnavailable
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
}

func (m metered) Capabilities() aiprovider.ProviderCapabilities {
	return m.provider.Capabilities()
}

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
		return m.budget.MeterAi(ctx, m.clock.Now(), tokens)
	})
}
