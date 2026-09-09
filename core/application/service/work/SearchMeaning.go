// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// SearchMeaning embeds what somebody typed, where that is possible at all (J-10, ADR-0050).
//
// **Its whole design is the answer to "and if it is not".** Semantic search is optional four times
// over - the extension may be absent, the workspace may have configured no provider, it may have
// configured one and not consented, and the provider may simply not answer today - and every one of
// those has the same right answer: search lexically. So this returns a vector or nothing, and never
// an error a caller has to interpret.
//
// That is also why the timeout is here and short. A search is something somebody is waiting for; a
// provider that takes four seconds has, for the purpose of this feature, not answered. The lexical
// half is complete on its own, so the cost of giving up early is a search that finds slightly less,
// and the cost of not giving up is a search that hangs.
type SearchMeaning struct {
	Providers AiProviders
	Semantic  SemanticSearch
	// UnitOfWork is needed for the one read: whether this installation has a store at all.
	UnitOfWork persistence.UnitOfWork
	// Timeout bounds the provider call. Zero takes the default below.
	Timeout time.Duration
}

// defaultMeaningTimeout is how long a search waits for a provider.
//
// Under a second, deliberately. This is the one AI call in the product that a person is waiting on,
// and the alternative to it is not an error but a complete lexical search - so the question is not
// "how long might a provider need" but "how long is a search allowed to take before finding less is
// better than finding it late".
const defaultMeaningTimeout = 800 * time.Millisecond

var _ QueryMeaning = SearchMeaning{}

// Of embeds the query, or answers nothing.
func (m SearchMeaning) Of(
	ctx context.Context, actor appshared.ActorContext, words string,
) ([]float32, error) {
	if m.Providers == nil || m.Semantic == nil || words == "" {
		return nil, nil
	}

	// The cheapest question first: an installation with no store cannot use a vector however good
	// its provider is, and asking a provider for one would be paying for an answer nothing can
	// read.
	var available bool
	if err := m.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			var err error
			available, err = m.Semantic.Available(ctx)
			return err
		}); err != nil {
		return nil, err
	}
	if !available {
		return nil, nil
	}

	provider, err := m.Providers.For(ctx, actor)
	if err != nil {
		// A provider that cannot be resolved - an unopenable key, a database that answered badly -
		// is a lexical search rather than a failed one. The resolver already answers NoopAi for
		// every ordinary reason; this covers the rest.
		return nil, nil
	}
	if !provider.Capabilities().Embedding {
		return nil, nil
	}

	timeout := m.Timeout
	if timeout <= 0 {
		timeout = defaultMeaningTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	answer, err := provider.Embed(bounded, []string{words})
	if err != nil || len(answer.Vectors) != 1 {
		// Every failure is the same answer, which is the port's own discipline applied to a read:
		// a slow provider, an open circuit, an exhausted budget and a refused key are one thing to
		// somebody who is waiting for search results.
		return nil, nil
	}
	return answer.Vectors[0], nil
}
