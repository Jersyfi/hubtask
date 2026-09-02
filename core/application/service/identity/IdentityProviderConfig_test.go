// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// providerStore is the configuration, in memory.
type providerStore struct {
	configured *domain.IdentityProvider
	sealed     cryptoport.Sealed
	deletes    int
}

func (s *providerStore) Upsert(
	_ context.Context, configured domain.IdentityProvider, sealed cryptoport.Sealed, now time.Time,
) (domain.IdentityProvider, error) {
	stored := configured
	if s.configured != nil {
		stored.Version = s.configured.Version + 1
		stored.UpdatedAt = now
	}
	s.configured, s.sealed = &stored, sealed
	return stored, nil
}

func (s *providerStore) Find(context.Context) (domain.IdentityProvider, error) {
	if s.configured == nil {
		return domain.IdentityProvider{}, shared.ErrNotFound.
			WithDetail("identity_provider.not_configured")
	}
	return *s.configured, nil
}

func (s *providerStore) FindWithSecret(
	context.Context,
) (domain.IdentityProvider, cryptoport.Sealed, error) {
	if s.configured == nil {
		return domain.IdentityProvider{}, cryptoport.Sealed{}, shared.ErrNotFound.
			WithDetail("identity_provider.not_configured")
	}
	return *s.configured, s.sealed, nil
}

func (s *providerStore) Delete(context.Context) (bool, error) {
	s.deletes++
	if s.configured == nil {
		return false, nil
	}
	s.configured = nil
	return true, nil
}

// relyingDouble stands in for the library. What matters to these tests is whether it was asked.
type relyingDouble struct {
	checked []string
	refuse  error
}

func (r *relyingDouble) Check(_ context.Context, issuer string) error {
	r.checked = append(r.checked, issuer)
	return r.refuse
}

func (r *relyingDouble) AuthorizationURL(
	context.Context, provider.Config, provider.Authorization,
) (string, error) {
	return "https://login.example.org/authorize", nil
}

func (r *relyingDouble) Exchange(
	context.Context, provider.Config, provider.Exchange,
) (provider.Identity, error) {
	return provider.Identity{}, nil
}

type providerFixture struct {
	writer  IdentityProviderWriter
	store   *providerStore
	relying *relyingDouble
	auth    *authorizer
	session *sessionFixture
}

func newProviderFixture(at time.Time) *providerFixture {
	session := mfaFixture(at)
	f := &providerFixture{
		store: &providerStore{}, relying: &relyingDouble{}, auth: &authorizer{}, session: session,
	}
	f.writer = IdentityProviderWriter{
		Session: session.writer, Providers: f.store, Relying: f.relying, Authorizer: f.auth,
	}
	return f
}

func configureCommand() ConfigureIdentityProviderCommand {
	return ConfigureIdentityProviderCommand{
		Issuer:              "https://login.example.org",
		ClientID:            "hubtask",
		ClientSecret:        secret.New("s3cr3t"),
		AllowedEmailDomains: []string{"Example.org"},
		Enabled:             true,
	}
}

func providerActor() appshared.ActorContext {
	actor := adminActor()
	actor.Scopes = append(actor.Scopes, "identity_provider:manage")
	return actor
}

// The order is the point: a workspace is not pointed at an issuer that answers nothing, and the
// refusal arrives while somebody is still looking at the form.
func TestTheProviderIsAskedToProveItExistsBeforeAnythingIsStored(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)
	f.relying.refuse = shared.ErrUnavailable.WithDetail("auth.provider_unreachable")

	_, err := ConfigureIdentityProvider{Writer: f.writer}.
		Execute(t.Context(), providerActor(), configureCommand())
	if err == nil {
		t.Fatal("an unreachable issuer was configured")
	}
	if len(f.relying.checked) != 1 {
		t.Errorf("discovery ran %d times", len(f.relying.checked))
	}
	if f.store.configured != nil {
		t.Error("the configuration was stored despite the refusal")
	}
	if len(f.session.audit.entries) != 0 {
		t.Error("a refused configuration was recorded as one that happened")
	}
}

// The secret is sealed on the way in, and what comes back out of the use case does not carry it.
func TestTheClientSecretIsSealedAndNeverAnswered(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)

	configured, err := ConfigureIdentityProvider{Writer: f.writer}.
		Execute(t.Context(), providerActor(), configureCommand())
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if f.store.sealed.IsZero() {
		t.Fatal("nothing was sealed")
	}
	if string(f.store.sealed.Ciphertext) == "s3cr3t" {
		t.Error("the secret reached the store in clear")
	}
	// The read shape has no field for it, which is the structural half of the promise.
	out := providerOutput(configured)
	for _, forbidden := range []string{"client_secret", "secret", "client_secret_enc"} {
		if _, held := out[forbidden]; held {
			t.Errorf("the answer carries %q", forbidden)
		}
	}
	if configured.AllowedEmailDomains[0] != "example.org" {
		t.Errorf("the domain was stored as %q, want it normalised", configured.AllowedEmailDomains[0])
	}
}

// Configuring and removing are both events a review looks for, and both name the workspace.
func TestConfiguringAndRemovingAreRecorded(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)

	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), configureCommand()); err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if err := (RemoveIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor()); err != nil {
		t.Fatalf("removing: %v", err)
	}

	if len(f.session.audit.entries) != 2 {
		t.Fatalf("the trail holds %d entries, want 2", len(f.session.audit.entries))
	}
	if got := f.session.audit.entries[0].Action; got != IdentityProviderConfiguredAction {
		t.Errorf("the first entry is %q", got)
	}
	if got := f.session.audit.entries[1].Action; got != IdentityProviderRemovedAction {
		t.Errorf("the second entry is %q", got)
	}
	for _, entry := range f.session.audit.entries {
		if entry.TargetID != tenant {
			t.Errorf("the entry targets %q, want the workspace", entry.TargetID)
		}
	}
}

// Removing what is not there is what the caller asked for, not a failure - and it records
// nothing, because nothing happened.
func TestRemovingAProviderThatIsNotThereIsNotAFailure(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))

	if err := (RemoveIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor()); err != nil {
		t.Fatalf("removing nothing: %v", err)
	}
	if len(f.session.audit.entries) != 0 {
		t.Error("removing nothing was recorded as an event")
	}
}

// Every one of the three asks the authoriser first, and the read is the only one that offers the
// auditor's alternative (A-4).
func TestTheAuthoriserIsAskedAndOnlyTheReadOffersTheAuditorsWay(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))

	_, _ = ConfigureIdentityProvider{Writer: f.writer}.
		Execute(t.Context(), providerActor(), configureCommand())
	_, _ = ReadIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor())
	_ = RemoveIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor())

	if len(f.auth.requests) != 3 {
		t.Fatalf("the authoriser was asked %d times, want 3", len(f.auth.requests))
	}
	for i, request := range f.auth.requests {
		if request.Permission != service.PermissionManageMembers {
			t.Errorf("request %d asks for %q", i, request.Permission)
		}
	}
	if f.auth.requests[0].Alternative != "" || f.auth.requests[2].Alternative != "" {
		t.Error("a write offers the auditor's read-only permission")
	}
	if f.auth.requests[1].Alternative != service.PermissionReadConfiguration {
		t.Error("the read does not accept the auditor's permission (A-4)")
	}
}

// A refused caller changes nothing, whichever of the three they called.
func TestARefusedCallerChangesNothing(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))
	f.auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), configureCommand()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("configuring answered %v", err)
	}
	if err := (RemoveIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("removing answered %v", err)
	}
	if f.store.configured != nil || f.store.deletes != 0 || len(f.relying.checked) != 0 {
		t.Error("a refused caller reached the store or the provider")
	}
}

// A configuration without a secret is refused before anything else happens: an empty one would
// seal to a valid envelope holding nothing, and the failure would surface at the first exchange.
func TestAProviderNeedsItsClientSecret(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))
	cmd := configureCommand()
	cmd.ClientSecret = secret.Secret{}

	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), cmd); err == nil {
		t.Fatal("a provider without a client secret was configured")
	}
	if len(f.relying.checked) != 0 {
		t.Error("discovery ran for a configuration that could never work")
	}
}
