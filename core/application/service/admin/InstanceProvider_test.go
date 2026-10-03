// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The control plane's half of the provider store (SI-10, ADR-0070 §2).
//
// What these tests are about is the level: the same writer the workspace's own half uses, under the
// scope that has no tenant, behind the scope and the operator register both - and a row that comes
// out calling itself the installation's rather than anybody's.

// instanceProviderStore is the collection at the installation's level, in memory.
type instanceProviderStore struct {
	rows   []domain.IdentityProvider
	sealed map[shared.ID]cryptoport.Sealed
}

func newInstanceProviderStore() *instanceProviderStore {
	return &instanceProviderStore{sealed: map[shared.ID]cryptoport.Sealed{}}
}

func (s *instanceProviderStore) List(context.Context) ([]domain.IdentityProvider, error) {
	return s.rows, nil
}

func (s *instanceProviderStore) Count(context.Context) (int, error) { return len(s.rows), nil }

func (s *instanceProviderStore) Find(_ context.Context, id shared.ID) (domain.IdentityProvider, error) {
	for _, row := range s.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
}

func (s *instanceProviderStore) FindWithSecret(
	ctx context.Context, id shared.ID,
) (domain.IdentityProvider, cryptoport.Sealed, error) {
	found, err := s.Find(ctx, id)
	return found, s.sealed[id], err
}

func (s *instanceProviderStore) Insert(
	_ context.Context, configured domain.IdentityProvider, sealed cryptoport.Sealed,
) (domain.IdentityProvider, error) {
	s.rows = append(s.rows, configured)
	s.sealed[configured.ID] = sealed
	return configured, nil
}

func (s *instanceProviderStore) Update(
	_ context.Context, configured domain.IdentityProvider,
	sealed *cryptoport.Sealed, now time.Time,
) (domain.IdentityProvider, bool, error) {
	for i, row := range s.rows {
		if row.ID != configured.ID {
			continue
		}
		stored := configured
		stored.Version = row.Version + 1
		stored.UpdatedAt = now
		s.rows[i] = stored
		if sealed != nil {
			s.sealed[stored.ID] = *sealed
		}
		return stored, true, nil
	}
	return domain.IdentityProvider{}, false, nil
}

// Reconfigure is never the installation's: its form still writes the offer. A call here is a
// defect the test should see.
func (s *instanceProviderStore) Reconfigure(
	context.Context, domain.IdentityProvider, *cryptoport.Sealed, time.Time,
) (domain.IdentityProvider, bool, error) {
	panic("the installation's form reconfigured without its switch")
}

func (s *instanceProviderStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	for i, row := range s.rows {
		if row.ID == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			delete(s.sealed, id)
			return true, nil
		}
	}
	return false, nil
}

var _ identityrepo.IdentityProviders = (*instanceProviderStore)(nil)

// reachableProvider stands in for the library: what matters here is whether it was asked.
type reachableProvider struct{ checked []string }

func (r *reachableProvider) Check(_ context.Context, issuer string) error {
	r.checked = append(r.checked, issuer)
	return nil
}

func (r *reachableProvider) AuthorizationURL(
	context.Context, provider.Config, provider.Authorization,
) (string, error) {
	return "", nil
}

func (r *reachableProvider) Exchange(
	context.Context, provider.Config, provider.Exchange,
) (provider.Identity, error) {
	return provider.Identity{}, nil
}

// plainEnvelope seals by labelling rather than by encrypting: what the tests here assert is that
// the purpose is the level's, not that the arithmetic works.
type plainEnvelope struct{ purposes []cryptoport.Purpose }

func (e *plainEnvelope) Seal(
	_ context.Context, value secret.Secret, purpose cryptoport.Purpose,
) (cryptoport.Sealed, error) {
	e.purposes = append(e.purposes, purpose)
	return cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("sealed:" + value.Reveal())}, nil
}

func (e *plainEnvelope) Open(
	_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose,
) (secret.Secret, error) {
	return secret.New(string(sealed.Ciphertext)), nil
}

func (e *plainEnvelope) Rewrap(
	_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose,
) (cryptoport.Sealed, error) {
	return sealed, nil
}

func (e *plainEnvelope) ActiveKeyID() string { return "k1" }

func (e *plainEnvelope) KeyIDs() []string { return []string{"k1"} }

func newInstanceProviderWriter(register *registerStore) (InstanceProviderWriter, *instanceProviderStore, *reachableProvider, *plainEnvelope) {
	instanceWriter, _, _ := newInstanceWriter(register)
	store := newInstanceProviderStore()
	relying := &reachableProvider{}
	envelope := &plainEnvelope{}
	return InstanceProviderWriter{
		Instance:  instanceWriter,
		Providers: store,
		Configure: identityservice.IdentityProviderWriter{
			Session: identityservice.SessionWriter{
				UnitOfWork: instanceWriter.UnitOfWork,
				Clock:      clock.Fixed(fixed),
				IDs:        &ids{},
				Encryptor:  envelope,
				Audit:      &auditSink{},
			},
			Providers:   store,
			Relying:     relying,
			RedirectURL: "https://hubtask.example/auth/callback",
		},
	}, store, relying, envelope
}

// A row written at this level belongs to no workspace, and says so: that is what every workspace's
// read policy admits and what its write policy refuses.
func TestTheInstallationsProviderBelongsToNoWorkspace(t *testing.T) {
	writer, store, relying, envelope := newInstanceProviderWriter(newRegister(operatorID))

	stored, err := ConfigureInstanceIdentityProvider{Writer: writer}.Execute(
		t.Context(), operator(), identityservice.ConfigureIdentityProviderCommand{
			Issuer: "https://accounts.google.com", ClientID: "hubtask", Kind: "GOOGLE",
			ClientSecret: secret.New("s3cr3t"),
		})
	if err != nil {
		t.Fatalf("configuring the installation's provider: %v", err)
	}
	if !stored.Installation() {
		t.Errorf("the row belongs to %q, want no workspace", stored.TenantID)
	}
	// The installation has no list of ways to sign in: its form is still the offer, and absent is
	// on. Only a workspace's own doors stopped switching (ADR-0076 §5, SC-21).
	if !stored.Enabled {
		t.Error("the installation's provider was added off")
	}
	if len(relying.checked) != 1 {
		t.Errorf("discovery ran %d times", len(relying.checked))
	}
	// The purpose is the level's: a zero workspace, which is what keeps a ciphertext sealed here
	// from opening inside somebody's workspace.
	if len(envelope.purposes) != 1 ||
		envelope.purposes[0] != identityservice.ClientSecretPurpose(shared.ID("")) {
		t.Errorf("sealed under %v", envelope.purposes)
	}

	// The write goes under the scope with no tenant. An installation scope is read-only by
	// construction, so it is the system scope - the same one `WriteInstanceSettings` uses.
	work, _ := writer.Instance.UnitOfWork.(*unitOfWork)
	held := false
	for _, scope := range work.scopes {
		held = held || scope == persistence.SystemScope()
	}
	if !held {
		t.Errorf("the write ran under %v, want the scope with no tenant", work.scopes)
	}

	if len(store.rows) != 1 {
		t.Fatalf("the store holds %d rows", len(store.rows))
	}
}

// The pair every operation at this level demands: the scope, then the register.
func TestTheInstanceProviderOperationsDemandTheRegisterAsWellAsTheScope(t *testing.T) {
	writer, store, relying, _ := newInstanceProviderWriter(newRegister(secondOperator))

	cmd := identityservice.ConfigureIdentityProviderCommand{
		Issuer: "https://accounts.google.com", ClientID: "hubtask", Kind: "GOOGLE",
		ClientSecret: secret.New("s3cr3t"),
	}
	if _, err := (ConfigureInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), cmd); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a writer outside the register answered %v", err)
	}
	if _, err := (ListInstanceIdentityProviders{Writer: writer}).
		Execute(t.Context(), operator()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader outside the register answered %v", err)
	}
	if err := (RemoveInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), shared.ID("01936f2a-7c1e-7000-8000-0000000000c1"), ""); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a removal outside the register answered %v", err)
	}
	if len(store.rows) != 0 || len(relying.checked) != 0 {
		t.Error("a refused caller reached the store or the provider")
	}
	if _, err := (ListInstanceIdentityProviders{Writer: writer}).
		Execute(t.Context(), anonymousActor()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("a caller with no credential answered %v", err)
	}
}

// Through the registry, the way a request arrives: the descriptor declares, the registry validates
// against that declaration, and the journal carries the act at the level no workspace's trail could.
func TestTheInstanceProviderUseCasesGoThroughTheRegistry(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))

	descriptors := []usecase.Descriptor{
		ConfigureInstanceIdentityProvider{Writer: writer}.Descriptor(),
		ListInstanceIdentityProviders{Writer: writer}.Descriptor(),
		RemoveInstanceIdentityProvider{Writer: writer}.Descriptor(),
	}
	registry, err := usecase.NewRegistry(nil, descriptors...)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	added, err := registry.Invoke(
		t.Context(), ConfigureInstanceIdentityProviderName, operator(), usecase.Input{
			"issuer":        "https://accounts.google.com",
			"kind":          "GOOGLE",
			"client_id":     "hubtask",
			"client_secret": "s3cr3t",
			"display_name":  "The platform",
			"enabled":       true,
		})
	if err != nil {
		t.Fatalf("adding through the registry: %v", err)
	}
	if added["scope"] != identityservice.ProviderScopeInstallation {
		t.Errorf("the answer calls the row %v", added["scope"])
	}
	id, _ := added["id"].(string)
	if id == "" {
		t.Fatal("the answer carries no identifier")
	}

	listed, err := registry.Invoke(
		t.Context(), ListInstanceIdentityProvidersName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the registry: %v", err)
	}
	if rows, held := listed["data"].([]usecase.Output); !held || len(rows) != 1 {
		t.Fatalf("the listing answered %v", listed)
	}

	if _, err := registry.Invoke(
		t.Context(), RemoveInstanceIdentityProviderName, operator(), usecase.Input{"id": id},
	); err != nil {
		t.Fatalf("removing through the registry: %v", err)
	}
	if len(store.rows) != 0 {
		t.Errorf("the removal left %d rows", len(store.rows))
	}
}
