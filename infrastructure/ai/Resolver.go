// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"
	"errors"
	"sync"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Resolver answers "which provider does this workspace use", which is a question with a database
// in it: a provider is per tenant (ai-first.md §2), so there is no one adapter the composition
// root can wire.
//
// It is an adapter and not an application service, and that is the seam it exists to keep. What
// the application layer holds is a port.Provider; what it never holds is a row, a key or a
// decision about which HTTP client to use.
//
// Four answers, and three of them are Noop:
//
//   - the workspace configured nothing;
//   - it configured NOOP, which is how AI is switched off without losing the configuration;
//   - it configured a provider and has not consented (`processing_allowed` false) - which is the
//     check ai-first.md §2 asks for "before every call", made here so that no call is ever built;
//   - it configured a provider and consented, which is the one case that produces an adapter.
//
// The fourth is also the only case that opens the sealed key, so an unconsenting workspace's
// credential is not decrypted on the way to deciding not to use it.
type Resolver struct {
	Providers  repository.AiProviders
	UnitOfWork persistence.UnitOfWork
	Encryptor  cryptoport.Encryptor
	Client     httpport.Port
	Clock      clock.Clock
	Meter      Meter
	// Breakers hands out one breaker per endpoint. Never nil in production.
	Breakers *BreakerPool
}

// For answers the provider of the actor's workspace.
//
// It never fails for a reason a caller has to handle differently: an unreachable database, an
// unopenable key and a workspace that chose nothing all produce the provider that refuses, because
// the caller's answer to all three is the same one - carry on without the suggestion. What differs
// is the log line, which is the adapter's business.
func (r Resolver) For(ctx context.Context, actor appshared.ActorContext) (port.Provider, error) {
	var (
		configured domain.AiProvider
		sealed     *cryptoport.Sealed
	)
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		configured, sealed, err = r.Providers.FindWithKey(ctx)
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return Noop{}, nil
		}
		return Noop{}, err
	}

	if configured.Kind == domain.AiNoop || !configured.ProcessingAllowed {
		return Noop{}, nil
	}

	var key secret.Secret
	if sealed != nil {
		opened, err := r.Encryptor.Open(ctx, *sealed,
			cryptoport.Purpose("ai_provider.api_key:"+actor.TenantID.String()))
		if err != nil {
			// A key sealed under a master key this installation no longer holds is a provider
			// that cannot be called. Noop rather than an error, for the reason above, and the
			// operator sees it in the sealing census rather than in a suggestion that failed.
			return Noop{}, nil
		}
		key = opened
	}

	adapter := OpenAiCompatible{
		Client: r.Client, Clock: r.Clock, Meter: r.Meter,
		BaseURL: configured.BaseURL, APIKey: key,
		CompletionModel: configured.CompletionModel, EmbeddingModel: configured.EmbeddingModel,
	}
	if r.Breakers != nil {
		adapter.Breaker = r.Breakers.For(configured.BaseURL)
	}
	if r.Meter == nil {
		adapter.Meter = noMeter{}
	}

	switch configured.Kind {
	case domain.AiOpenAiCompatible:
		return adapter, nil
	case domain.AiOllama:
		// The local adapter is J-04's. Until it lands, a workspace that configured Ollama gets
		// the provider that refuses rather than an OpenAI-compatible call to an endpoint that
		// speaks something else - which would be a confusing failure instead of an honest one.
		return Noop{}, nil
	default:
		return Noop{}, nil
	}
}

// BreakerPool hands out one circuit breaker per endpoint, bounded.
//
// Per endpoint rather than one for the whole installation, because a provider is per tenant: a
// single breaker would let one workspace's dead endpoint switch off everybody's suggestions, which
// is exactly the cross-tenant interference multi-tenancy.md §4 is about.
//
// Bounded, because the key comes from configuration and configuration comes from tenants. The map
// is cleared wholesale when it grows past the cap rather than evicted one by one: an eviction
// policy here would be a cache algorithm guarding a few hundred small structs, and losing a
// breaker's state costs one extra call to an endpoint that is probably still down.
type BreakerPool struct {
	// New builds a breaker for an endpoint. Injected, so the pool does not import the resilience
	// adapter and the composition root keeps the thresholds in one place.
	New func(endpoint string) Breaker
	// Cap is how many endpoints the pool holds before it starts again. Zero means 256.
	Cap int

	mu       sync.Mutex
	breakers map[string]Breaker
}

// For answers the breaker of one endpoint.
func (p *BreakerPool) For(endpoint string) Breaker {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.breakers == nil {
		p.breakers = make(map[string]Breaker)
	}
	if held, seen := p.breakers[endpoint]; seen {
		return held
	}

	limit := p.Cap
	if limit <= 0 {
		limit = 256
	}
	if len(p.breakers) >= limit {
		p.breakers = make(map[string]Breaker)
	}

	breaker := p.New(endpoint)
	p.breakers[endpoint] = breaker
	return breaker
}

// Size is how many endpoints this process has called. What the health probe reads to tell
// "nothing is using AI here" from "something is, and it works".
func (p *BreakerPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.breakers)
}

// Open reports how many endpoints are currently cut off, for the health probe and the gauge. It is
// a count rather than a list: an endpoint is a tenant's configuration, and a metric labelled by one
// would grow a series per customer (rule 10).
func (p *BreakerPool) Open(isOpen func(Breaker) bool) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := 0
	for _, breaker := range p.breakers {
		if isOpen(breaker) {
			count++
		}
	}
	return count
}
