// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	service "github.com/Jersyfi/hubtask/core/application/service/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// PG-8: third-country AI without explicit confirmation is refused (ADR-0018 decision 7,
// data-protection.md §6 and §10).
//
// **It was a tripwire until J-02 and is a check now.** There was nothing to gate before: no AI
// provider surface existed, and a gate written against that absence would have been a green nobody
// may read as a check. What arrived with J-02 is the surface the tripwire was watching for - a
// provider a workspace configures, with a declared jurisdiction - so the tripwire is replaced by
// the measure it named.
//
// Three things are asserted, because ADR-0018 decision 7 is three sentences. The transfer is
// refused without the installation's confirmation. The confirmation is the *installation's* and
// defaults to off, which is what makes it the deliberate friction the ADR asks for rather than a
// formality a workspace administrator clicks past. And the use is audited with the provider, the
// region and the model - the ADR names all three, and the fourth thing it names, the purpose, is
// the action.
//
// It runs in `make gate-privacy` and needs no database: the refusal is the application layer's,
// which is where every authorisation and every rule of this kind lives (rule 2, ADR-0005).

func TestPG8AThirdCountryProviderIsRefusedWithoutTheConfirmation(t *testing.T) {
	writer, _ := aiWriter(false)

	_, err := service.ConfigureAiProvider{Writer: writer}.
		Execute(context.Background(), aiActor(), thirdCountryProvider())

	if err == nil {
		t.Fatal("a provider outside the EEA was configured with no confirmation from the operator")
	}
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("the refusal was %v, want a forbidden - a transfer nobody confirmed is not a "+
			"malformed request", err)
	}
	if got := shared.AsError(err).DetailCode; got != "ai.third_country_not_confirmed" {
		t.Errorf("detail code %q, want ai.third_country_not_confirmed", got)
	}
}

// Nothing may be written or sealed by a refused configuration. A row that exists but is "not
// allowed" is a transfer waiting for somebody to flip a switch, which is not what "refused" means.
func TestPG8ARefusedTransferWritesAndSealsNothing(t *testing.T) {
	writer, store := aiWriter(false)

	_, _ = service.ConfigureAiProvider{Writer: writer}.
		Execute(context.Background(), aiActor(), thirdCountryProvider())

	if store.stored != nil {
		t.Error("a refused configuration was written all the same")
	}
	if store.sealed > 0 {
		t.Error("a refused configuration sealed its key on the way to being refused")
	}
}

func TestPG8TheConfirmationLetsTheTransferThrough(t *testing.T) {
	writer, store := aiWriter(true)

	configured, err := service.ConfigureAiProvider{Writer: writer}.
		Execute(context.Background(), aiActor(), thirdCountryProvider())

	if err != nil {
		t.Fatalf("the operator confirmed the transfer and it was refused anyway: %v", err)
	}
	if configured.Jurisdiction != domain.AiThirdCountry {
		t.Errorf("jurisdiction %q, want %q", configured.Jurisdiction, domain.AiThirdCountry)
	}
	if store.stored == nil {
		t.Fatal("the configuration was accepted and not written")
	}
}

// The three jurisdictions that are not a third-country transfer need no confirmation, and an
// installation that never set the flag must still be able to use a European or a local provider.
func TestPG8OnlyAThirdCountryNeedsIt(t *testing.T) {
	for _, jurisdiction := range []domain.AiJurisdiction{domain.AiEEA, domain.AiAdequacy} {
		t.Run(string(jurisdiction), func(t *testing.T) {
			writer, _ := aiWriter(false)
			command := thirdCountryProvider()
			command.Jurisdiction = jurisdiction

			if _, err := (service.ConfigureAiProvider{Writer: writer}).
				Execute(context.Background(), aiActor(), command); err != nil {
				t.Fatalf("%q was refused on an installation that confirmed nothing: %v",
					jurisdiction, err)
			}
		})
	}
}

// The confirmation is the installation's, and it is off unless it is said.
//
// The field is named through the type rather than searched for in the text, so that renaming it
// away fails to compile rather than fails to match: a check satisfied by a mention in a comment is
// a check a rename walks past. What is read as text is the *variable*, in the adapter that reads
// it, because what data-protection.md §6 promises an operator is a name they type into an
// environment - and a promise about a name is a promise about a string.
func TestPG8TheConfirmationIsTheInstallationsAndDefaultsToOff(t *testing.T) {
	var unconfigured env.AIConfig
	if unconfigured.AllowThirdCountryTransfer {
		t.Error("an installation that configured nothing confirms a third-country transfer; " +
			"ADR-0018 decision 7 asks for deliberate friction, and a default is the opposite")
	}

	adapter := readFile(t, "../../infrastructure/environment/EnvConfig.go")
	if !strings.Contains(adapter, "HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER") {
		t.Error("nothing reads HUBTASK_AI_ALLOW_THIRD_COUNTRY_TRANSFER, which is the name " +
			"data-protection.md §6 promises operators; a documented setting and an implemented " +
			"one have drifted apart")
	}

	writer, _ := aiWriter(false)
	if writer.ThirdCountryConfirmed {
		t.Error("the zero value of the writer confirms a third-country transfer")
	}
}

// ADR-0018 decision 7: "the use is audited with the provider, region, model, and purpose". The
// first three are the entry's changes and the fourth is the action, which is what an action name
// is for - and the key is in none of it.
func TestPG8TheTransferIsAuditedWithProviderRegionAndModel(t *testing.T) {
	writer, store := aiWriter(true)

	if _, err := (service.ConfigureAiProvider{Writer: writer}).
		Execute(context.Background(), aiActor(), thirdCountryProvider()); err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("%d audit entries, want exactly one", len(store.entries))
	}

	entry := store.entries[0]
	if entry.Action != service.AiProviderConfiguredAction {
		t.Errorf("action %q, want %q - the purpose is the action",
			entry.Action, service.AiProviderConfiguredAction)
	}

	// Changes() has already masked; an OPEN field arrives as {"to": value}, which is what makes
	// this readable at all - a SENSITIVE one would be a hash and a SECRET one absent.
	recorded := map[string]string{}
	for field, masked := range entry.Changes {
		shape, held := masked.(map[string]any)
		if !held {
			t.Fatalf("the entry records %s as %T, not a masked change", field, masked)
		}
		recorded[field], _ = shape["to"].(string)
	}
	for field, want := range map[string]string{
		"kind":             string(domain.AiOpenAiCompatible),
		"jurisdiction":     string(domain.AiThirdCountry),
		"completion_model": "a-model",
	} {
		if recorded[field] != want {
			t.Errorf("the entry records %s = %q, want %q", field, recorded[field], want)
		}
	}

	for field, value := range recorded {
		if strings.Contains(value, aiSecretKey) {
			t.Errorf("the audit entry carries the API key in %s", field)
		}
	}
}

// The fixtures. Small on purpose: what PG-8 checks is a rule of the application layer, so the
// pieces around it are the smallest things that let the rule run.

const aiSecretKey = "a-key-nobody-should-see"

func aiActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:     appshared.ActorUser,
		TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
	}
}

func thirdCountryProvider() service.ConfigureAiProviderCommand {
	return service.ConfigureAiProviderCommand{
		Kind:            domain.AiOpenAiCompatible,
		BaseURL:         "https://api.example.org/v1",
		CompletionModel: "a-model",
		APIKey:          secret.New(aiSecretKey),
		APIKeyPresent:   true,
		Jurisdiction:    domain.AiThirdCountry,
	}
}

func aiWriter(confirmed bool) (service.AiProviderWriter, *aiStore) {
	store := &aiStore{}
	return service.AiProviderWriter{
		Providers: store, Authorizer: aiAllow{}, Encryptor: store,
		Audit: store, UnitOfWork: aiDirect{}, Clock: aiClock{},
		ThirdCountryConfirmed: confirmed,
	}, store
}

// aiStore is the repository, the encryptor and the audit sink in one, so that a test can assert
// "nothing was written and nothing was sealed" from a single place.
type aiStore struct {
	stored  *domain.AiProvider
	sealed  int
	entries []audit.Entry
}

func (s *aiStore) Upsert(
	_ context.Context, provider domain.AiProvider, _ *crypto.Sealed, _ time.Time,
) (domain.AiProvider, error) {
	stored := provider
	stored.Version = 1
	s.stored = &stored
	return stored, nil
}

func (s *aiStore) UpsertKeepingKey(
	ctx context.Context, provider domain.AiProvider, now time.Time,
) (domain.AiProvider, error) {
	return s.Upsert(ctx, provider, nil, now)
}

func (s *aiStore) Find(context.Context) (domain.AiProvider, error) {
	if s.stored == nil {
		return domain.AiProvider{}, shared.ErrNotFound.WithDetail("ai.not_configured")
	}
	return *s.stored, nil
}

func (s *aiStore) FindWithKey(ctx context.Context) (domain.AiProvider, *crypto.Sealed, error) {
	found, err := s.Find(ctx)
	return found, nil, err
}

func (s *aiStore) Delete(context.Context) (bool, error) {
	had := s.stored != nil
	s.stored = nil
	return had, nil
}

func (s *aiStore) RewrapKey(context.Context, crypto.Sealed, string) (bool, error) {
	return false, nil
}

func (s *aiStore) Seal(
	_ context.Context, _ secret.Secret, _ crypto.Purpose,
) (crypto.Sealed, error) {
	s.sealed++
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte("sealed")}, nil
}

func (s *aiStore) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (s *aiStore) Rewrap(
	_ context.Context, sealed crypto.Sealed, _ crypto.Purpose,
) (crypto.Sealed, error) {
	return sealed, nil
}

func (s *aiStore) ActiveKeyID() string { return "k1" }

func (s *aiStore) KeyIDs() []string { return []string{"k1"} }

func (s *aiStore) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

var (
	_ repository.AiProviders = (*aiStore)(nil)
	_ crypto.Encryptor       = (*aiStore)(nil)
	_ audit.Sink             = (*aiStore)(nil)
)

// aiAllow lets everything through: what PG-8 is about is the transfer, and an authorisation
// failure would hide the refusal this gate exists to see.
type aiAllow struct{}

func (aiAllow) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

type aiDirect struct{}

func (aiDirect) Within(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

func (aiDirect) WithinReadOnly(ctx context.Context, _ persistence.Scope, run func(context.Context) error) error {
	return run(ctx)
}

type aiClock struct{}

func (aiClock) Now() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC) }
