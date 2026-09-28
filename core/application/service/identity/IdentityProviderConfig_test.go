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

// providerStore is the collection, in memory. Plural since SI-10, and it keeps the installation's
// rows beside a workspace's - because the read the use cases make sees both and the write does not.
type providerStore struct {
	rows    []domain.IdentityProvider
	sealed  map[shared.ID]cryptoport.Sealed
	deletes int
	// scope is the workspace the fake is standing in for. A row of another tenant is invisible to
	// it, and a row of no tenant is readable and not writable - which is what the policy does.
	scope shared.ID
}

func newProviderStore(scope shared.ID) *providerStore {
	return &providerStore{sealed: map[shared.ID]cryptoport.Sealed{}, scope: scope}
}

// visible is the read policy: this level's rows and the installation's.
func (s *providerStore) visible() []domain.IdentityProvider {
	found := []domain.IdentityProvider{}
	for _, row := range s.rows {
		if row.TenantID == s.scope || row.Installation() {
			found = append(found, row)
		}
	}
	return found
}

// writable is the write policy: this level's rows only.
func (s *providerStore) writable(id shared.ID) int {
	for i, row := range s.rows {
		if row.ID == id && row.TenantID == s.scope {
			return i
		}
	}
	return -1
}

func (s *providerStore) List(context.Context) ([]domain.IdentityProvider, error) {
	return s.visible(), nil
}

func (s *providerStore) Count(context.Context) (int, error) {
	counted := 0
	for _, row := range s.rows {
		if row.TenantID == s.scope {
			counted++
		}
	}
	return counted, nil
}

func (s *providerStore) Find(_ context.Context, id shared.ID) (domain.IdentityProvider, error) {
	for _, row := range s.visible() {
		if row.ID == id {
			return row, nil
		}
	}
	return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
}

func (s *providerStore) FindWithSecret(
	_ context.Context, id shared.ID,
) (domain.IdentityProvider, cryptoport.Sealed, error) {
	found, err := s.Find(context.Background(), id)
	if err != nil {
		return domain.IdentityProvider{}, cryptoport.Sealed{}, err
	}
	return found, s.sealed[id], nil
}

func (s *providerStore) Insert(
	_ context.Context, configured domain.IdentityProvider, sealed cryptoport.Sealed,
) (domain.IdentityProvider, error) {
	s.rows = append(s.rows, configured)
	s.sealed[configured.ID] = sealed
	return configured, nil
}

func (s *providerStore) Update(
	_ context.Context, configured domain.IdentityProvider,
	sealed *cryptoport.Sealed, now time.Time,
) (domain.IdentityProvider, bool, error) {
	at := s.writable(configured.ID)
	if at < 0 {
		return domain.IdentityProvider{}, false, nil
	}
	stored := configured
	stored.Version = s.rows[at].Version + 1
	stored.CreatedAt = s.rows[at].CreatedAt
	stored.UpdatedAt = now
	s.rows[at] = stored
	if sealed != nil {
		s.sealed[stored.ID] = *sealed
	}
	return stored, true, nil
}

func (s *providerStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	s.deletes++
	at := s.writable(id)
	if at < 0 {
		return false, nil
	}
	s.rows = append(s.rows[:at], s.rows[at+1:]...)
	delete(s.sealed, id)
	return true, nil
}

// only answers the single row, for a test that does not care which key it got.
func (s *providerStore) only(t *testing.T) domain.IdentityProvider {
	t.Helper()
	if len(s.rows) != 1 {
		t.Fatalf("the store holds %d providers, want one", len(s.rows))
	}
	return s.rows[0]
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
		store: newProviderStore(tenant), relying: &relyingDouble{},
		auth: &authorizer{}, session: session,
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
	if len(f.store.rows) != 0 {
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
	sealed := f.store.sealed[configured.ID]
	if sealed.IsZero() {
		t.Fatal("nothing was sealed")
	}
	if string(sealed.Ciphertext) == "s3cr3t" {
		t.Error("the secret reached the store in clear")
	}
	// The read shape has no field for it, which is the structural half of the promise.
	out := ProviderOutput(configured)
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
		Execute(t.Context(), providerActor(), f.store.only(t).ID); err != nil {
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
	// The row is the target since SI-10: there are several, and which one changed is the first
	// thing a reader of the trail needs.
	for _, entry := range f.session.audit.entries {
		if entry.TargetID.IsZero() {
			t.Error("the entry names no provider")
		}
		if entry.TenantID != tenant {
			t.Errorf("the entry belongs to %q, want the workspace", entry.TenantID)
		}
	}
}

// Removing what is not there is what the caller asked for, not a failure - and it records
// nothing, because nothing happened.
func TestRemovingAProviderThatIsNotThereIsNotAFailure(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))

	if err := (RemoveIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), shared.ID("01936f2a-7c1e-7000-8000-00000000ffff")); err != nil {
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
	_, _ = ListIdentityProviders{Writer: f.writer}.Execute(t.Context(), providerActor())
	_ = RemoveIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), f.store.only(t).ID)

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
		Execute(t.Context(), providerActor(), shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("removing answered %v", err)
	}
	if len(f.store.rows) != 0 || f.store.deletes != 0 || len(f.relying.checked) != 0 {
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
