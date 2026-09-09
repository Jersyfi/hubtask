// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

// The semantic half of the search, which is almost entirely a set of ways of not having one. Every
// test below asserts the same thing in a different situation: no vector, no error, and the search
// carries on lexically.

func meaningHarness(world *embeddingWorld) SearchMeaning {
	return SearchMeaning{Providers: world, Semantic: world, UnitOfWork: &unitOfWork{}}
}

// The one case where there is a vector.
func TestAQueryIsEmbeddedWhereEverythingIsInPlace(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}

	vector, err := meaningHarness(world).Of(t.Context(), itemActor(), "the thing about invoices")
	if err != nil {
		t.Fatalf("embedding the query failed: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("the query embedded to %v", vector)
	}
	if len(world.embedded) != 1 || world.embedded[0][0] != "the thing about invoices" {
		t.Errorf("the provider was asked %v", world.embedded)
	}
}

// Every way of not having a vector, and all of them answer nothing rather than an error. That is
// the whole design: a search must not fail because somebody else's machine is slow, unconfigured or
// switched off.
func TestEveryReasonNotToEmbedIsALexicalSearchRatherThanAnError(t *testing.T) {
	for _, c := range []struct {
		name  string
		world *embeddingWorld
		words string
		asked bool
	}{
		{
			name:  "the installation has no embedding store",
			world: &embeddingWorld{embedding: true, model: "embed-3", vectors: [][]float32{{0.1}}},
			words: "invoices",
		},
		{
			name:  "the workspace has no provider that embeds",
			world: &embeddingWorld{available: true},
			words: "invoices",
		},
		{
			name:  "the provider refuses - an open circuit, a spent budget, a refused key",
			world: &embeddingWorld{available: true, embedding: true, model: "embed-3", embedErr: aiprovider.ErrUnavailable},
			words: "invoices",
			asked: true,
		},
		{
			name:  "the provider answers the wrong number of vectors",
			world: &embeddingWorld{available: true, embedding: true, model: "embed-3", vectors: [][]float32{{0.1}, {0.2}}},
			words: "invoices",
			asked: true,
		},
		{
			name:  "there are no words to embed",
			world: &embeddingWorld{available: true, embedding: true, model: "embed-3", vectors: [][]float32{{0.1}}},
			words: "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			vector, err := meaningHarness(c.world).Of(t.Context(), itemActor(), c.words)
			if err != nil {
				t.Fatalf("a search was failed rather than made lexical: %v", err)
			}
			if vector != nil {
				t.Errorf("a vector came back anyway: %v", vector)
			}
			if asked := len(c.world.embedded) > 0; asked != c.asked {
				t.Errorf("the provider was asked = %v, want %v", asked, c.asked)
			}
		})
	}
}

// A provider that is merely slow is a provider that has not answered: the timeout is short because
// the alternative to waiting is a complete lexical search rather than an error.
func TestASlowProviderIsALexicalSearch(t *testing.T) {
	world := &embeddingWorld{available: true, embedding: true, model: "embed-3"}
	handler := meaningHarness(world)
	handler.Providers = slowProvider{embeddingProvider{world: world}}
	handler.Timeout = time.Millisecond

	vector, err := handler.Of(t.Context(), itemActor(), "invoices")
	if err != nil {
		t.Fatalf("a slow provider failed the search: %v", err)
	}
	if vector != nil {
		t.Errorf("a vector came back from a provider that never answered: %v", vector)
	}
}

// A provider that cannot even be resolved - an unopenable key, a database that answered badly - is
// a lexical search too. The resolver answers NoopAi for every ordinary reason; this is the rest.
func TestAProviderThatCannotBeResolvedIsALexicalSearch(t *testing.T) {
	world := &embeddingWorld{available: true, embedding: true, model: "embed-3"}
	handler := meaningHarness(world)
	handler.Providers = brokenProviders{}

	vector, err := handler.Of(t.Context(), itemActor(), "invoices")
	if err != nil {
		t.Fatalf("an unresolvable provider failed the search: %v", err)
	}
	if vector != nil {
		t.Errorf("a vector came back anyway: %v", vector)
	}
}

// A search that is not wired for meaning at all - an installation running without the seam - is the
// search this product had before J-10, rather than a nil dereference.
func TestASearchWithoutTheSeamIsTheSearchItAlwaysWas(t *testing.T) {
	vector, err := SearchMeaning{}.Of(t.Context(), itemActor(), "invoices")
	if err != nil || vector != nil {
		t.Fatalf("an unwired seam answered %v, %v", vector, err)
	}
}

// slowProvider answers nothing at all until the caller gives up, which is the only interesting
// thing a provider can do to a search: the timeout is what turns it into a lexical one. It is its
// own resolver as well, because there is nothing about resolving it that is interesting.
type slowProvider struct{ embeddingProvider }

func (p slowProvider) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return p, nil
}

func (p slowProvider) Embed(
	ctx context.Context, texts []string,
) (aiprovider.EmbeddingResult, error) {
	<-ctx.Done()
	return aiprovider.EmbeddingResult{}, ctx.Err()
}

type brokenProviders struct{}

func (brokenProviders) For(context.Context, appshared.ActorContext) (aiprovider.Provider, error) {
	return nil, errors.New("the key could not be opened")
}
