// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The refusal of a third-country transfer is PG-8's, in test/privacy, because it is a gate. What
// is here is everything else the three use cases promise.

func TestConfiguringSealsTheKeyUnderTheWorkspacesOwnPurpose(t *testing.T) {
	writer, store := providerWriter()

	if _, err := (ConfigureAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor(), providerCommand()); err != nil {
		t.Fatalf("configuring: %v", err)
	}

	if store.sealedUnder != AiKeyPurpose(providerActor().TenantID) {
		t.Errorf("the key was sealed under %q, want the workspace's own",
			store.sealedUnder)
	}
	if store.sealed == nil {
		t.Fatal("the configuration was stored with no envelope")
	}
}

// Absent keeps and present-but-empty clears. It is the one field of this input where the
// difference between the two is the whole meaning, and a caller who changed a model must not lose
// the key by not resending it.
func TestAnAbsentKeyKeepsTheStoredOneAndAnEmptyOneClearsIt(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		amend     func(*ConfigureAiProviderCommand)
		wantKeeps bool
		wantSeals bool
	}{
		{"a key that was sent", func(*ConfigureAiProviderCommand) {}, false, true},
		{"no key at all", func(c *ConfigureAiProviderCommand) {
			c.APIKeyPresent, c.APIKey = false, secret.Secret{}
		}, true, false},
		{"an empty key", func(c *ConfigureAiProviderCommand) {
			c.APIKey = secret.New("")
		}, false, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			writer, store := providerWriter()
			command := providerCommand()
			testCase.amend(&command)

			if _, err := (ConfigureAiProvider{Writer: writer}).
				Execute(context.Background(), providerActor(), command); err != nil {
				t.Fatalf("configuring: %v", err)
			}
			if store.keptKey != testCase.wantKeeps {
				t.Errorf("kept the stored key = %v, want %v", store.keptKey, testCase.wantKeeps)
			}
			if (store.sealed != nil) != testCase.wantSeals {
				t.Errorf("sealed a key = %v, want %v", store.sealed != nil, testCase.wantSeals)
			}
		})
	}
}

// Nothing reaches the store unless the actor may write, which is rule 2: the check is here and not
// in the adapter, and the adapter is never asked.
func TestAnUnauthorisedActorReachesNothing(t *testing.T) {
	writer, store := providerWriter()
	writer.Authorizer = providerRefuse{}

	if _, err := (ConfigureAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor(), providerCommand()); err == nil {
		t.Fatal("an unauthorised actor configured a provider")
	}
	if store.stored != nil || store.sealed != nil || len(store.entries) > 0 {
		t.Error("a refused caller reached the store, the encryptor or the trail")
	}
}

func TestReadingAnswersTheConfigurationAndNeverTheKey(t *testing.T) {
	writer, store := providerWriter()
	if _, err := (ConfigureAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor(), providerCommand()); err != nil {
		t.Fatalf("configuring: %v", err)
	}

	found, err := ReadAiProvider{Writer: writer}.Execute(context.Background(), providerActor())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !found.HasAPIKey {
		t.Error("a stored key is not reported at all, so nobody can tell whether one is set")
	}
	if store.keyReads > 0 {
		t.Error("an ordinary read opened the envelope; only the adapter that calls may")
	}

	// The output map is what REST, MCP and automation all render from, so the key's absence is
	// asserted there rather than in one adapter.
	out := aiProviderOutput(found)
	for _, forbidden := range []string{"api_key", "api_key_enc", "key"} {
		if _, held := out[forbidden]; held {
			t.Errorf("the read shape carries %q", forbidden)
		}
	}
}

func TestReadingAWorkspaceThatChoseNothingSaysSo(t *testing.T) {
	writer, _ := providerWriter()

	_, err := ReadAiProvider{Writer: writer}.Execute(context.Background(), providerActor())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("reading an unconfigured workspace answered %v, want a not-found", err)
	}
}

// Removing something that is not there is not a failure: the caller asked for it to be gone and it
// is. What it must not do is write a trail entry about a removal that did not happen.
func TestRemovingNothingIsNotAFailureAndRecordsNothing(t *testing.T) {
	writer, store := providerWriter()

	if err := (RemoveAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor()); err != nil {
		t.Fatalf("removing an unconfigured provider: %v", err)
	}
	if len(store.entries) != 0 {
		t.Errorf("%d audit entries for a removal that removed nothing", len(store.entries))
	}
}

func TestRemovingAConfiguredProviderRecordsIt(t *testing.T) {
	writer, store := providerWriter()
	if _, err := (ConfigureAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor(), providerCommand()); err != nil {
		t.Fatalf("configuring: %v", err)
	}

	if err := (RemoveAiProvider{Writer: writer}).
		Execute(context.Background(), providerActor()); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if store.stored != nil {
		t.Error("the configuration survived its own removal")
	}
	if len(store.entries) != 2 || store.entries[1].Action != AiProviderRemovedAction {
		t.Errorf("the trail records %d entries ending in %v", len(store.entries), store.entries)
	}
}

// The resealer moves the envelope and leaves the configuration alone, which is what makes a
// rotation finishable: an operator retiring a key has not changed anybody's provider.
func TestTheResealerMovesTheEnvelopeAndNotTheConfiguration(t *testing.T) {
	store := &providerStore{active: "k2"}
	store.stored = &domain.AiProvider{Kind: domain.AiOpenAiCompatible, Version: 7}
	store.sealed = &crypto.Sealed{KeyID: "k1", Ciphertext: []byte("x")}

	outcome, err := AiProviderResealer{Providers: store, Encryptor: store}.
		Reseal(context.Background(), providerActor().TenantID)
	if err != nil || outcome.Rewrapped != 1 {
		t.Fatalf("outcome %+v, %v", outcome, err)
	}
	if store.stored.Version != 7 {
		t.Errorf("the version moved to %d; a re-seal is not a change to the configuration",
			store.stored.Version)
	}
	if store.rewrappedUnder != AiKeyPurpose(providerActor().TenantID) {
		t.Errorf("rewrapped under %q, want the workspace's own", store.rewrappedUnder)
	}
}

func TestAProviderWithNoKeyHasNothingToReseal(t *testing.T) {
	store := &providerStore{active: "k2"}
	store.stored = &domain.AiProvider{Kind: domain.AiOllama}

	outcome, err := AiProviderResealer{Providers: store, Encryptor: store}.
		Reseal(context.Background(), providerActor().TenantID)
	if err != nil || outcome.Rewrapped != 0 || outcome.Skipped != 0 {
		t.Fatalf("a provider that needs no key produced %+v, %v", outcome, err)
	}
}

// The fixtures.

func providerActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:     appshared.ActorUser,
		TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"),
	}
}

func providerCommand() ConfigureAiProviderCommand {
	return ConfigureAiProviderCommand{
		Kind:            domain.AiOpenAiCompatible,
		BaseURL:         "https://api.example.org/v1",
		CompletionModel: "a-model",
		APIKey:          secret.New("a-key"),
		APIKeyPresent:   true,
		Jurisdiction:    domain.AiEEA,
	}
}

func providerWriter() (AiProviderWriter, *providerStore) {
	store := &providerStore{active: "k1"}
	return AiProviderWriter{
		Providers: store, Authorizer: providerAllow{}, Encryptor: store,
		Audit: store, UnitOfWork: providerDirect{}, Clock: providerClock{},
	}, store
}

type providerStore struct {
	active         string
	stored         *domain.AiProvider
	sealed         *crypto.Sealed
	sealedUnder    crypto.Purpose
	rewrappedUnder crypto.Purpose
	keptKey        bool
	keyReads       int
	entries        []audit.Entry
}

func (s *providerStore) Upsert(
	_ context.Context, provider domain.AiProvider, sealed *crypto.Sealed, _ time.Time,
) (domain.AiProvider, error) {
	stored := provider
	stored.HasAPIKey = sealed != nil
	s.stored, s.sealed = &stored, sealed
	return stored, nil
}

func (s *providerStore) UpsertKeepingKey(
	_ context.Context, provider domain.AiProvider, _ time.Time,
) (domain.AiProvider, error) {
	s.keptKey = true
	stored := provider
	stored.HasAPIKey = s.sealed != nil
	s.stored = &stored
	return stored, nil
}

func (s *providerStore) Find(context.Context) (domain.AiProvider, error) {
	if s.stored == nil {
		return domain.AiProvider{}, shared.ErrNotFound.WithDetail("ai.not_configured")
	}
	return *s.stored, nil
}

func (s *providerStore) FindWithKey(ctx context.Context) (domain.AiProvider, *crypto.Sealed, error) {
	s.keyReads++
	found, err := s.Find(ctx)
	return found, s.sealed, err
}

func (s *providerStore) Delete(context.Context) (bool, error) {
	had := s.stored != nil
	s.stored, s.sealed = nil, nil
	return had, nil
}

func (s *providerStore) RewrapKey(_ context.Context, sealed crypto.Sealed, _ string) (bool, error) {
	s.sealed = &sealed
	return true, nil
}

func (s *providerStore) Seal(
	_ context.Context, _ secret.Secret, purpose crypto.Purpose,
) (crypto.Sealed, error) {
	s.sealedUnder = purpose
	return crypto.Sealed{KeyID: s.active, Ciphertext: []byte("sealed")}, nil
}

func (s *providerStore) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (s *providerStore) Rewrap(
	_ context.Context, sealed crypto.Sealed, purpose crypto.Purpose,
) (crypto.Sealed, error) {
	s.rewrappedUnder = purpose
	return crypto.Sealed{KeyID: s.active, Ciphertext: sealed.Ciphertext}, nil
}

func (s *providerStore) ActiveKeyID() string { return s.active }
func (s *providerStore) KeyIDs() []string    { return []string{s.active} }

func (s *providerStore) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

var (
	_ repository.AiProviders = (*providerStore)(nil)
	_ crypto.Encryptor       = (*providerStore)(nil)
	_ audit.Sink             = (*providerStore)(nil)
)

type providerAllow struct{}

func (providerAllow) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

type providerRefuse struct{}

func (providerRefuse) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return shared.ErrForbidden
}

type providerDirect struct{}

func (providerDirect) Within(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

func (providerDirect) WithinReadOnly(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

type providerClock struct{}

func (providerClock) Now() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) }
