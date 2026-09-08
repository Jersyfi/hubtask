// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
)

// Three of the four answers are the provider that refuses, and the fourth is the only one that
// opens a key. Which means an unconsenting workspace's credential is never decrypted on the way to
// deciding not to use it.
func TestOnlyAConfiguredAndConsentingWorkspaceGetsAProvider(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		store     *resolverStore
		wantNoop  bool
		wantOpens int
	}{
		{"nothing configured", &resolverStore{missing: true}, true, 0},
		{"AI switched off", &resolverStore{
			configured: domain.AiProvider{Kind: domain.AiNoop},
		}, true, 0},
		{"configured and not consented", &resolverStore{
			configured: domain.AiProvider{
				Kind: domain.AiOpenAiCompatible, BaseURL: "https://api.example.org/v1",
				CompletionModel: "a-model", ProcessingAllowed: false,
			},
			sealed: &cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("x")},
		}, true, 0},
		{"configured and consented", &resolverStore{
			configured: domain.AiProvider{
				Kind: domain.AiOpenAiCompatible, BaseURL: "https://api.example.org/v1",
				CompletionModel: "a-model", ProcessingAllowed: true,
			},
			sealed: &cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("x")},
		}, false, 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resolver := resolverFor(testCase.store)

			provider, err := resolver.For(context.Background(), resolverActor())
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if _, isNoop := provider.(ai.Noop); isNoop != testCase.wantNoop {
				t.Errorf("got %T, want a noop = %v", provider, testCase.wantNoop)
			}
			if testCase.store.opens != testCase.wantOpens {
				t.Errorf("the key was opened %d times, want %d",
					testCase.store.opens, testCase.wantOpens)
			}
		})
	}
}

// The kind decides the adapter, and a local provider gets the local one. A key is never passed to
// it: Ollama has no credential of its own, and an operator who put a proxy in front configures the
// OpenAI-compatible adapter, which is where a key belongs.
func TestTheKindDecidesTheAdapter(t *testing.T) {
	for _, testCase := range []struct {
		kind domain.AiProviderKind
		want string
	}{
		{domain.AiOpenAiCompatible, ai.OpenAiCompatibleKind},
		{domain.AiOllama, ai.OllamaKind},
	} {
		t.Run(string(testCase.kind), func(t *testing.T) {
			store := &resolverStore{
				configured: domain.AiProvider{
					Kind: testCase.kind, BaseURL: "http://models.internal:11434",
					CompletionModel: "a-model", ProcessingAllowed: true,
				},
				sealed: &cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("x")},
			}

			provider, err := resolverFor(store).For(context.Background(), resolverActor())
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if got := provider.Capabilities().Kind; got != testCase.want {
				t.Errorf("kind %q, want %q", got, testCase.want)
			}
			if local, isLocal := provider.(ai.Ollama); isLocal {
				if _, holdsKey := any(local).(interface{ APIKey() string }); holdsKey {
					t.Error("the local adapter has somewhere to keep a key")
				}
			}
		})
	}
}

// An unreachable database, an unopenable key and a workspace that chose nothing produce one
// behaviour, because a caller's answer to all three is the same one.
func TestAKeySealedUnderAKeyThisInstallationLostRefusesRatherThanFails(t *testing.T) {
	store := &resolverStore{
		configured: domain.AiProvider{
			Kind: domain.AiOpenAiCompatible, BaseURL: "https://api.example.org/v1",
			CompletionModel: "a-model", ProcessingAllowed: true,
		},
		sealed:   &cryptoport.Sealed{KeyID: "gone", Ciphertext: []byte("x")},
		openFail: true,
	}

	provider, err := resolverFor(store).For(context.Background(), resolverActor())
	if err != nil {
		t.Fatalf("an unopenable key answered %v, want the refusing provider", err)
	}
	if _, isNoop := provider.(ai.Noop); !isNoop {
		t.Errorf("got %T, want the provider that refuses", provider)
	}
}

// The key is opened under the workspace's own purpose, so a ciphertext lifted into another
// workspace's row does not open (E-02).
func TestTheKeyIsOpenedUnderTheWorkspacesOwnPurpose(t *testing.T) {
	store := &resolverStore{
		configured: domain.AiProvider{
			Kind: domain.AiOpenAiCompatible, BaseURL: "https://api.example.org/v1",
			CompletionModel: "a-model", ProcessingAllowed: true,
		},
		sealed: &cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("x")},
	}

	if _, err := resolverFor(store).For(context.Background(), resolverActor()); err != nil {
		t.Fatalf("resolving: %v", err)
	}
	want := cryptoport.Purpose("ai_provider.api_key:" + resolverActor().TenantID.String())
	if store.openedUnder != want {
		t.Errorf("opened under %q, want %q", store.openedUnder, want)
	}
}

// A provider that needs no key - which is the ordinary shape of a local model - is a provider, not
// a half-written row.
func TestAProviderWithNoKeyStillResolves(t *testing.T) {
	store := &resolverStore{configured: domain.AiProvider{
		Kind: domain.AiOpenAiCompatible, BaseURL: "https://models.internal/v1",
		EmbeddingModel: "an-embedding-model", ProcessingAllowed: true,
	}}

	provider, err := resolverFor(store).For(context.Background(), resolverActor())
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	capabilities := provider.Capabilities()
	if capabilities.Kind != ai.OpenAiCompatibleKind || !capabilities.Embedding || capabilities.Completion {
		t.Errorf("capabilities %+v", capabilities)
	}
}

// The fixtures.

func resolverActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:     appshared.ActorUser,
		TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000d1"),
	}
}

func resolverFor(store *resolverStore) ai.Resolver {
	return ai.Resolver{
		Providers: store, UnitOfWork: resolverDirect{}, Encryptor: store,
		Client: &recordingClient{}, Clock: fixedClock{}, Meter: &countingMeter{},
		Breakers: &ai.BreakerPool{New: func(string) ai.Breaker { return nil }},
	}
}

type resolverStore struct {
	configured  domain.AiProvider
	sealed      *cryptoport.Sealed
	missing     bool
	openFail    bool
	opens       int
	openedUnder cryptoport.Purpose
}

func (s *resolverStore) Upsert(context.Context, domain.AiProvider, *cryptoport.Sealed, time.Time) (domain.AiProvider, error) {
	return domain.AiProvider{}, nil
}

func (s *resolverStore) UpsertKeepingKey(context.Context, domain.AiProvider, time.Time) (domain.AiProvider, error) {
	return domain.AiProvider{}, nil
}

func (s *resolverStore) Find(context.Context) (domain.AiProvider, error) {
	if s.missing {
		return domain.AiProvider{}, shared.ErrNotFound.WithDetail("ai.not_configured")
	}
	return s.configured, nil
}

func (s *resolverStore) FindWithKey(ctx context.Context) (domain.AiProvider, *cryptoport.Sealed, error) {
	found, err := s.Find(ctx)
	return found, s.sealed, err
}

func (s *resolverStore) Delete(context.Context) (bool, error) { return false, nil }

func (s *resolverStore) RewrapKey(context.Context, cryptoport.Sealed, string) (bool, error) {
	return false, nil
}

func (s *resolverStore) Seal(context.Context, secret.Secret, cryptoport.Purpose) (cryptoport.Sealed, error) {
	return cryptoport.Sealed{}, nil
}

func (s *resolverStore) Open(
	_ context.Context, _ cryptoport.Sealed, purpose cryptoport.Purpose,
) (secret.Secret, error) {
	s.opens, s.openedUnder = s.opens+1, purpose
	if s.openFail {
		return secret.Secret{}, errors.New("unknown key")
	}
	return secret.New("a-key"), nil
}

func (s *resolverStore) Rewrap(_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose) (cryptoport.Sealed, error) {
	return sealed, nil
}

func (s *resolverStore) ActiveKeyID() string { return "k1" }
func (s *resolverStore) KeyIDs() []string    { return []string{"k1"} }

type resolverDirect struct{}

func (resolverDirect) Within(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

func (resolverDirect) WithinReadOnly(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

var _ port.Provider = ai.Noop{}
