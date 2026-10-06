// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
	// afterFind runs once a Find has answered: a test's way to change the row between a use case's
	// read and its write, which is what another administrator's request does.
	afterFind func()
	// lists counts the reads of the whole list - the ways in, as the fallback reads them.
	lists int
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
	s.lists++
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
			if s.afterFind != nil {
				s.afterFind()
			}
			return row, nil
		}
	}
	return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
}

func (s *providerStore) FindWithSecret(
	ctx context.Context, id shared.ID,
) (domain.IdentityProvider, cryptoport.Sealed, error) {
	found, err := s.Find(ctx, id)
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

// Reconfigure is Update with the row's own switch kept, as the statement's COALESCE keeps it.
func (s *providerStore) Reconfigure(
	ctx context.Context, configured domain.IdentityProvider,
	sealed *cryptoport.Sealed, now time.Time,
) (domain.IdentityProvider, bool, error) {
	if at := s.writable(configured.ID); at >= 0 {
		configured.Enabled = s.rows[at].Enabled
	}
	return s.Update(ctx, configured, sealed, now)
}

// SetWithdrawal writes only the installation's rows, and only from the installation's own scope -
// which a fake standing in for a workspace is not.
func (s *providerStore) SetWithdrawal(
	_ context.Context, id shared.ID, at, now time.Time,
) (domain.IdentityProvider, bool, error) {
	if !s.scope.IsZero() {
		return domain.IdentityProvider{}, false, nil
	}
	for i, row := range s.rows {
		if row.ID == id && row.Installation() {
			row.WithdrawAt = at
			if at.IsZero() {
				row.Enabled = true
			}
			row.UpdatedAt, row.Version = now, row.Version+1
			s.rows[i] = row
			return row, true, nil
		}
	}
	return domain.IdentityProvider{}, false, nil
}

// Delete asks what the statement asks: an installation's row still offered and used stays.
func (s *providerStore) Delete(_ context.Context, id shared.ID, now time.Time) (bool, error) {
	s.deletes++
	at := s.writable(id)
	if at < 0 || s.rows[at].RemovableAt(now) != nil {
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
		Execute(t.Context(), providerActor(), f.store.only(t).ID, ""); err != nil {
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
		Execute(t.Context(), providerActor(), shared.ID("01936f2a-7c1e-7000-8000-00000000ffff"), ""); err != nil {
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
	_ = RemoveIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), f.store.only(t).ID, "")

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
		Execute(t.Context(), providerActor(), shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), ""); !errors.Is(err, shared.ErrForbidden) {
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

// Every provider use case goes through the registry the way a request does: the descriptor
// declares, the registry validates against that declaration, and the handler is reached with an
// input it accepts.
//
// It is one test rather than four because the failure it catches is one thing: an input key a
// descriptor does not declare is refused by the registry before the handler ever sees it, and a
// test that called the handler directly would never meet that refusal. That is how a use case comes
// to pass every unit test and answer 422 to every request.
func TestTheProviderUseCasesGoThroughTheRegistry(t *testing.T) {
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)
	f.writer.RedirectURL = "https://hubtask.example/auth/callback"

	descriptors := []usecase.Descriptor{
		ConfigureIdentityProvider{Writer: f.writer}.Descriptor(),
		ListIdentityProviders{Writer: f.writer}.Descriptor(),
		RemoveIdentityProvider{Writer: f.writer}.Descriptor(),
		ListIdentityProviderPresets{Writer: f.writer}.Descriptor(),
	}
	registry, err := usecase.NewRegistry(nil, descriptors...)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	added, err := registry.Invoke(t.Context(), ConfigureIdentityProviderName, providerActor(), usecase.Input{
		"issuer":                "https://login.example.org",
		"client_id":             "hubtask",
		"client_secret":         "s3cr3t",
		"display_name":          "Staff directory",
		"provisioning":          "DOMAINS",
		"position":              1,
		"allowed_email_domains": []any{"example.org"},
	})
	if err != nil {
		t.Fatalf("adding through the registry: %v", err)
	}
	if added["display_name"] != "Staff directory" || added["scope"] != ProviderScopeWorkspace {
		t.Errorf("the answer is %v", added)
	}
	id, _ := added["id"].(string)
	if id == "" {
		t.Fatal("the answer carries no identifier, so nothing could be replaced or removed")
	}

	// Replacing without a secret keeps the sealed one, which is the contract's promise and the
	// reason `client_secret` is not a required field.
	replaced, err := registry.Invoke(t.Context(), ConfigureIdentityProviderName, providerActor(), usecase.Input{
		"id":           id,
		"issuer":       "https://login.example.org",
		"client_id":    "hubtask",
		"display_name": "The other name",
		"enabled":      false,
	})
	if err != nil {
		t.Fatalf("replacing through the registry: %v", err)
	}
	if replaced["display_name"] != "The other name" || replaced["enabled"] != false {
		t.Errorf("the replacement answered %v", replaced)
	}
	if f.store.sealed[f.store.only(t).ID].IsZero() {
		t.Error("a replacement without a secret lost the one that was sealed")
	}

	listed, err := registry.Invoke(t.Context(), ListIdentityProvidersName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the registry: %v", err)
	}
	rows, held := listed["data"].([]usecase.Output)
	if !held || len(rows) != 1 {
		t.Fatalf("the listing answered %v", listed)
	}

	// The presets carry this installation's own callback, rendered into the instructions by
	// whichever client shows them - the code travels, never the sentence (rule 8).
	presets, err := registry.Invoke(t.Context(), ListIdentityProviderPresetsName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading the presets: %v", err)
	}
	presetRows, held := presets["data"].([]usecase.Output)
	if !held || len(presetRows) != len(domain.ProviderPresets()) {
		t.Fatalf("the presets answered %v", presets)
	}
	for _, preset := range presetRows {
		if preset["redirect_uri"] != "https://hubtask.example/auth/callback" {
			t.Errorf("a preset carries the callback %v", preset["redirect_uri"])
		}
		if preset["instructions"] == "" {
			t.Errorf("the %v preset carries no instructions", preset["kind"])
		}
	}

	if _, err := registry.Invoke(t.Context(), RemoveIdentityProviderName, providerActor(), usecase.Input{
		"id": id,
	}); err != nil {
		t.Fatalf("removing through the registry: %v", err)
	}
	if len(f.store.rows) != 0 {
		t.Errorf("the removal left %d rows", len(f.store.rows))
	}
}

// A public provider may only be INVITED_ONLY, and the refusal arrives from the use case rather than
// from the store: nothing is written and the provider is never asked to prove it exists.
func TestAPublicProviderIsHeldToInvitedOnly(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))

	cmd := configureCommand()
	cmd.Issuer = "https://accounts.google.com"
	cmd.Kind = "GOOGLE"
	cmd.Provisioning = "ANY"

	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), cmd); err == nil {
		t.Fatal("a public provider was configured to provision anybody")
	}
	if len(f.store.rows) != 0 || len(f.relying.checked) != 0 {
		t.Error("a refused configuration reached the store or the provider")
	}
}

// The bound on the collection is the workspace's own rows: an installation's are not a workspace's
// to be limited by, and the count the check reads says so.
func TestAWorkspaceMayNotConfigureMoreThanTheBound(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))

	for i := range MaxProvidersPerWorkspace {
		cmd := configureCommand()
		cmd.Issuer = "https://login" + strconv.Itoa(i) + ".example.org"
		if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
			Execute(t.Context(), providerActor(), cmd); err != nil {
			t.Fatalf("configuring provider %d: %v", i, err)
		}
	}

	cmd := configureCommand()
	cmd.Issuer = "https://one-too-many.example.org"
	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), cmd); err == nil {
		t.Error("a workspace configured one provider past the bound")
	}
}

// Which provider may vouch for this workspace's people is a sign-in rule, and changing it asks for a
// fresh proof (ADR-0071's addendum, E2): without it, an administrator's stolen session - or an
// administrator - could point a provider they control at every account here and stay unnoticed
// until the trail is read.
func TestChangingAWayInAsksForAFreshProof(t *testing.T) {
	f := newProviderFixture(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC))
	proof := &stepUpFake{}
	f.writer.StepUp = proof

	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), configureCommand()); err == nil {
		t.Fatal("a provider was configured without a fresh proof")
	}
	if len(f.store.rows) != 0 {
		t.Fatalf("a refused change stored %d rows", len(f.store.rows))
	}

	proof.satisfied = true
	command := configureCommand()
	command.StepUpToken = "hbt_stu_fresh"
	stored, err := (ConfigureIdentityProvider{Writer: f.writer}).Execute(t.Context(), providerActor(), command)
	if err != nil {
		t.Fatalf("configuring with the proof: %v", err)
	}
	if got := proof.presented[len(proof.presented)-1]; got != "hbt_stu_fresh" {
		t.Errorf("the proof presented was %q", got)
	}

	// Removing is a change to a way in as much as adding one.
	proof.satisfied = false
	if err := (RemoveIdentityProvider{Writer: f.writer}).
		Execute(t.Context(), providerActor(), stored.ID, ""); err == nil {
		t.Error("a provider was removed without a fresh proof")
	}
}
