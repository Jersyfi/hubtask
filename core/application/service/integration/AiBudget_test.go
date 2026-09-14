// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The per-tenant AI budget (J-15). What is asked here is the shape of the degradation - a workspace
// over its budget looks exactly like one with no provider - and the order of the two calls, which
// is the whole design: a budget cannot be reserved, so it is checked before and written after.

var budgetNow = time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)

type budgetWorld struct {
	room     bool
	roomErr  error
	metered  []int64
	meterErr error
	// completion and embedding are what the wrapped provider answers.
	usage   aiprovider.Usage
	callErr error
	calls   int
	// inside says whether the meter was written with a transaction open, which is where it has to
	// be: the provider call is outside one, and the ledger write is a write.
	openTransactions int
	meteredInside    bool
	// measures is what the provider answers to the width question; measurable false hands out a
	// provider that cannot be asked at all.
	measures   int
	measurable bool
}

func (w *budgetWorld) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	if w.measures > 0 && !w.measurable {
		// The world was told a width but not made measurable: the test flipped it off.
		return unmeasurable{inner: budgetProvider{world: w}}, nil
	}
	return budgetProvider{world: w}, nil
}

// MeasureEmbedding is the width question, answered from the world (#569).
func (p budgetProvider) MeasureEmbedding(context.Context) (int, error) { return p.world.measures, nil }

// unmeasurable is a provider without the optional method, which is what the interface assertion
// has to answer zero for. A field rather than an embedding, or the method would be promoted.
type unmeasurable struct{ inner budgetProvider }

func (u unmeasurable) Capabilities() aiprovider.ProviderCapabilities {
	return u.inner.Capabilities()
}
func (u unmeasurable) Complete(ctx context.Context, r aiprovider.CompletionRequest) (aiprovider.CompletionResult, error) {
	return u.inner.Complete(ctx, r)
}
func (u unmeasurable) Embed(ctx context.Context, t []string) (aiprovider.EmbeddingResult, error) {
	return u.inner.Embed(ctx, t)
}

func (w *budgetWorld) AiTokens(context.Context, string, time.Time) (bool, error) {
	return w.room, w.roomErr
}

func (w *budgetWorld) MeterAi(_ context.Context, _ time.Time, tokens int64) error {
	if w.openTransactions > 0 {
		w.meteredInside = true
	}
	w.metered = append(w.metered, tokens)
	return w.meterErr
}

func (w *budgetWorld) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	w.openTransactions++
	defer func() { w.openTransactions-- }()
	return fn(ctx)
}

func (w *budgetWorld) WithinReadOnly(
	ctx context.Context, s persistence.Scope, fn func(context.Context) error,
) error {
	return w.Within(ctx, s, fn)
}

type budgetProvider struct{ world *budgetWorld }

func (p budgetProvider) Capabilities() aiprovider.ProviderCapabilities {
	return aiprovider.ProviderCapabilities{
		Kind: "stub", Completion: true, Embedding: true,
		CompletionModel: "answer-1", EmbeddingModel: "embed-1",
	}
}

func (p budgetProvider) Complete(
	context.Context, aiprovider.CompletionRequest,
) (aiprovider.CompletionResult, error) {
	p.world.calls++
	if p.world.callErr != nil {
		return aiprovider.CompletionResult{}, p.world.callErr
	}
	return aiprovider.CompletionResult{Text: "{}", Usage: p.world.usage}, nil
}

func (p budgetProvider) Embed(
	context.Context, []string,
) (aiprovider.EmbeddingResult, error) {
	p.world.calls++
	if p.world.callErr != nil {
		return aiprovider.EmbeddingResult{}, p.world.callErr
	}
	return aiprovider.EmbeddingResult{Vectors: [][]float32{{0.1}}, Usage: p.world.usage}, nil
}

func budgeted(world *budgetWorld) Budgeted {
	return Budgeted{
		Providers: world, Budget: world, UnitOfWork: world, Clock: clock.Fixed(budgetNow),
	}
}

func budgetActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
}

// A workspace over its budget gets a provider that refuses exactly as an absent one does. That is
// the whole degradation, and it is why nothing downstream needed changing: every caller already
// deals with a provider that cannot do something.
func TestAWorkspaceOverItsBudgetLooksLikeOneWithNoProvider(t *testing.T) {
	world := &budgetWorld{room: false}

	provider, err := budgeted(world).For(t.Context(), budgetActor())
	if err != nil {
		t.Fatalf("resolving was refused: %v", err)
	}

	capabilities := provider.Capabilities()
	if capabilities.Enabled() || capabilities.Completion || capabilities.Embedding {
		t.Errorf("an exhausted budget answered a provider that can do things: %+v", capabilities)
	}

	if _, err := provider.Complete(t.Context(), aiprovider.CompletionRequest{}); !errors.Is(
		err, shared.ErrUnavailable) {
		t.Errorf("completing over budget answered %v, want the one refusal", err)
	}
	if _, err := provider.Embed(t.Context(), []string{"x"}); !errors.Is(
		err, shared.ErrUnavailable) {
		t.Errorf("embedding over budget answered %v, want the one refusal", err)
	}
	if world.calls != 0 {
		t.Errorf("%d calls reached the provider over budget", world.calls)
	}
}

// With room, the call goes through and what it cost is written afterwards - both halves of the
// tokens the provider reported, because a budget that counted only the answer would be a budget a
// long prompt walks past.
func TestACallWithRoomIsMeteredByWhatItCost(t *testing.T) {
	world := &budgetWorld{room: true, usage: aiprovider.Usage{InputTokens: 900, OutputTokens: 100}}

	provider, err := budgeted(world).For(t.Context(), budgetActor())
	if err != nil {
		t.Fatalf("resolving was refused: %v", err)
	}
	if _, err := provider.Complete(t.Context(), aiprovider.CompletionRequest{}); err != nil {
		t.Fatalf("completing failed: %v", err)
	}

	if len(world.metered) != 1 || world.metered[0] != 1000 {
		t.Errorf("metered %v, want the prompt and the answer together", world.metered)
	}
	// The ledger write is a write, and the provider call it follows happened outside a
	// transaction - so the meter has to open one of its own.
	if !world.meteredInside {
		t.Error("the tally was written outside a transaction")
	}
}

// A call that failed meters nothing: a provider that refused charged nothing, and a budget counting
// refusals would tighten on itself.
func TestAFailedCallCostsNothing(t *testing.T) {
	world := &budgetWorld{room: true, callErr: aiprovider.ErrUnavailable,
		usage: aiprovider.Usage{InputTokens: 900}}

	provider, _ := budgeted(world).For(t.Context(), budgetActor())
	if _, err := provider.Embed(t.Context(), []string{"x"}); err == nil {
		t.Fatal("a refusing provider answered")
	}

	if len(world.metered) != 0 {
		t.Errorf("a failed call was billed: %v", world.metered)
	}
}

// A provider that reports no usage - a local Ollama often does not - meters nothing, and that is
// honest rather than a hole: what it spends is the operator's own electricity.
func TestAProviderThatReportsNothingCostsNothing(t *testing.T) {
	world := &budgetWorld{room: true}

	provider, _ := budgeted(world).For(t.Context(), budgetActor())
	if _, err := provider.Complete(t.Context(), aiprovider.CompletionRequest{}); err != nil {
		t.Fatalf("completing failed: %v", err)
	}
	if len(world.metered) != 0 {
		t.Errorf("a call nobody priced was billed: %v", world.metered)
	}
}

// The wrapped provider is otherwise the workspace's own: the budget adds a bound, it does not
// change what a provider can do or what it answers.
func TestWithRoomTheProviderIsTheWorkspacesOwn(t *testing.T) {
	world := &budgetWorld{room: true}

	provider, _ := budgeted(world).For(t.Context(), budgetActor())
	capabilities := provider.Capabilities()

	if capabilities.CompletionModel != "answer-1" || capabilities.EmbeddingModel != "embed-1" {
		t.Errorf("the budget changed what the provider says it is: %+v", capabilities)
	}
}

// A budget that cannot be read is an error rather than a silent refusal. Every other failure in
// this file degrades, and this one does not - because "we could not tell whether you have room" is
// not the same statement as "you have none", and answering the second would make a database blip
// look like a spent budget to an operator reading their quota page.
func TestABudgetThatCannotBeReadIsAnError(t *testing.T) {
	world := &budgetWorld{roomErr: errors.New("the settings document would not open")}

	if _, err := budgeted(world).For(t.Context(), budgetActor()); err == nil {
		t.Fatal("an unreadable budget answered a provider")
	}
}

// An installation with no budget wired is the resolver unchanged, which is what a build without the
// quota machinery would be.
func TestWithoutABudgetTheResolverIsUntouched(t *testing.T) {
	world := &budgetWorld{room: false}
	unbudgeted := budgeted(world)
	unbudgeted.Budget = nil

	provider, err := unbudgeted.For(t.Context(), budgetActor())
	if err != nil {
		t.Fatalf("resolving was refused: %v", err)
	}
	if !provider.Capabilities().Enabled() {
		t.Error("a build with no budget refused a configured provider")
	}
}

// The budget wrapper forwards the width question to a provider that can answer it, and answers
// zero for one that cannot - and meters nothing either way, because no budget counts a question
// about a model (#569). Without this test, dropping the forwarding would send every pass back to
// the after-call path and every other test would still pass.
func TestTheBudgetWrapperForwardsTheWidthQuestionAndMetersNothing(t *testing.T) {
	world := &budgetWorld{room: true, measures: 3072, measurable: true}

	provider, err := budgeted(world).For(t.Context(), budgetActor())
	if err != nil {
		t.Fatalf("resolving was refused: %v", err)
	}
	measured, can := provider.(aiprovider.Measured)
	if !can {
		t.Fatal("a metered provider cannot be asked its width")
	}
	width, err := measured.MeasureEmbedding(t.Context())
	if err != nil || width != 3072 {
		t.Fatalf("the question answered %d, %v", width, err)
	}
	if len(world.metered) != 0 {
		t.Errorf("the question was metered: %v", world.metered)
	}

	// And a provider that cannot say answers zero rather than refusing.
	world.measurable = false
	provider, _ = budgeted(world).For(t.Context(), budgetActor())
	if width, err := provider.(aiprovider.Measured).MeasureEmbedding(t.Context()); err != nil || width != 0 {
		t.Errorf("an unmeasurable provider answered %d, %v", width, err)
	}
}
