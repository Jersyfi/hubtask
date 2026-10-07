// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The switch, both levels and the guard (ADR-0070 §2).
//
// What these assert is the one thing two stores make easy to get wrong: which store answered. For a
// workspace's own row the switch is the row; for one the installation offers, the row must come
// back untouched and the workspace's settings must carry the change — because the row belongs to
// the installation, and a workspace that could write it would be writing everybody's.

// offerFixture is the provider store, the workspace store and the one verb over both.
type offerFixture struct {
	*providerFixture
	workspaces *workspaceStore
	offer      OfferIdentityProvider
}

func newOfferFixture(at time.Time) *offerFixture {
	providers := newProviderFixture(at)
	workspaces := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{
			ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
			DefaultLocale: "en", DefaultTimeZone: "UTC",
		},
		Version: 4,
	}}
	return &offerFixture{
		providerFixture: providers,
		workspaces:      workspaces,
		offer:           OfferIdentityProvider{Writer: providers.writer, Workspaces: workspaces},
	}
}

// own puts a row of this workspace's own into the store and answers its identifier.
func (f *offerFixture) own(t *testing.T, enabled bool) shared.ID {
	t.Helper()
	id := shared.MustParseID("11111111-1111-4111-8111-111111111111")
	f.store.rows = append(f.store.rows, domain.IdentityProvider{
		ID: id, TenantID: tenant, Issuer: "https://login.example.org", ClientID: "hubtask",
		DisplayName: "Example", Kind: domain.KindGeneric,
		Provisioning: domain.ProvisionInvitedOnly, Enabled: enabled, Version: 1,
	})
	return id
}

// installation puts a row the installation offers every workspace into the store.
func (f *offerFixture) installation(t *testing.T) shared.ID {
	t.Helper()
	id := shared.MustParseID("22222222-2222-4222-8222-222222222222")
	f.store.rows = append(f.store.rows, domain.IdentityProvider{
		ID: id, Issuer: "https://login.platform.example", ClientID: "hubtask",
		DisplayName: "The platform", Kind: domain.KindGeneric,
		Provisioning: domain.ProvisionInvitedOnly, Enabled: true, Version: 1,
	})
	return id
}

// The concept's sentence, as a test: offered everywhere, on nowhere. Switching it on here writes
// the workspace's settings and leaves the installation's row exactly as it was.
func TestTakingAnInstallationsProviderWritesTheWorkspaceAndNotTheRow(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	f.own(t, true) // another way in, so the guard is not what is being measured here
	id := f.installation(t)

	switched, err := f.offer.Execute(t.Context(), providerActor(), id, true, "")
	if err != nil {
		t.Fatalf("switching on: %v", err)
	}
	if !switched.OfferedHere {
		t.Error("the answer does not say it is a way in here")
	}
	if !f.workspaces.row.Settings.Offers(id) {
		t.Error("the workspace's settings do not carry the offer")
	}
	// The row is the installation's. Not "unchanged as far as this workspace can tell" - unchanged.
	for _, row := range f.store.rows {
		if row.ID == id && row.Version != 1 {
			t.Errorf("the installation's row was written: version %d", row.Version)
		}
	}
	if len(f.workspaces.updates) != 1 {
		t.Fatalf("%d workspace writes, want one", len(f.workspaces.updates))
	}
	if f.workspaces.updates[0].expected != 4 {
		t.Errorf("the write guarded on version %d, want the row's", f.workspaces.updates[0].expected)
	}
}

// And giving it back is the same write in the other direction, again without touching the row.
func TestGivingAnInstallationsProviderBackClearsOnlyTheWorkspacesSwitch(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	f.own(t, true)
	id := f.installation(t)
	f.workspaces.row.Settings = f.workspaces.row.Settings.WithOffer(id, true)

	switched, err := f.offer.Execute(t.Context(), providerActor(), id, false, "")
	if err != nil {
		t.Fatalf("switching off: %v", err)
	}
	if switched.OfferedHere {
		t.Error("the answer still says it is a way in here")
	}
	if f.workspaces.row.Settings.Offers(id) {
		t.Error("the workspace's settings still carry the offer")
	}
}

// The workspace's own row is the other store, and there the switch *is* the row.
func TestSwitchingTheWorkspacesOwnProviderWritesTheRow(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)
	f.installation(t)
	f.workspaces.row.Settings = f.workspaces.row.Settings.WithOffer(
		shared.MustParseID("22222222-2222-4222-8222-222222222222"), true)

	switched, err := f.offer.Execute(t.Context(), providerActor(), id, false, "")
	if err != nil {
		t.Fatalf("switching off: %v", err)
	}
	if switched.Enabled || switched.OfferedHere {
		t.Error("the row was answered as still on")
	}
	if len(f.workspaces.updates) != 0 {
		t.Error("the workspace's settings were written for a row that is its own")
	}
}

// The guard. A workspace whose only method is a provider, and that provider switched off, is a
// workspace nobody can reach — so the switch is refused rather than obeyed.
func TestTheLastWayInCannotBeSwitchedOff(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)
	onlyProvider := []string{domain.MethodOidc}
	f.workspaces.row.Settings.SignIn.Methods = &onlyProvider

	_, err := f.offer.Execute(t.Context(), providerActor(), id, false, "")
	if err == nil {
		t.Fatal("the only way in was switched off")
	}
	if detail := shared.AsError(err).DetailCode; detail != "identity_provider.last_way_in" {
		t.Errorf("refused with %q", detail)
	}
	if f.store.rows[0].Enabled != true {
		t.Error("the row was written despite the refusal")
	}
}

// A password is a way in. The same workspace with the product's own methods may switch its
// provider off, because somebody can still sign in tomorrow.
func TestAProviderMayBeSwitchedOffWhileAPasswordOpensTheWorkspace(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)

	if _, err := f.offer.Execute(t.Context(), providerActor(), id, false, ""); err != nil {
		t.Fatalf("switching off: %v", err)
	}
}

// So is another provider that is on here — including one the installation offers and this
// workspace took, which is the case the two stores make easy to miss.
func TestAnotherWayInMayBeAnInstallationsProviderThisWorkspaceTook(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)
	offered := f.installation(t)
	onlyProvider := []string{domain.MethodOidc}
	f.workspaces.row.Settings.SignIn.Methods = &onlyProvider
	f.workspaces.row.Settings = f.workspaces.row.Settings.WithOffer(offered, true)

	if _, err := f.offer.Execute(t.Context(), providerActor(), id, false, ""); err != nil {
		t.Fatalf("switching off while another provider is on here: %v", err)
	}

	// And with that one *not* taken, the same arrangement is the last way in after all: an offer
	// nobody accepted is not a way in, which is the difference between offered and on.
	g := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	other := g.own(t, true)
	g.installation(t)
	g.workspaces.row.Settings.SignIn.Methods = &onlyProvider

	if _, err := g.offer.Execute(t.Context(), providerActor(), other, false, ""); err == nil {
		t.Fatal("an offer nobody took was counted as a way in")
	}
}

// The trail carries the direction and the level, and no secret.
func TestSwitchingIsRecorded(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	f.own(t, true)
	id := f.installation(t)

	if _, err := f.offer.Execute(t.Context(), providerActor(), id, true, ""); err != nil {
		t.Fatalf("switching on: %v", err)
	}
	if len(f.session.audit.entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(f.session.audit.entries))
	}
	entry := f.session.audit.entries[0]
	if entry.Action != IdentityProviderOfferedAction {
		t.Errorf("recorded as %q", entry.Action)
	}
	if entry.TargetID != id {
		t.Error("the entry names another provider")
	}
	// `Changes` masks by classification, so an open field arrives as {"to": …} rather than flat.
	direction, held := entry.Changes["offered_here"].(map[string]any)
	if !held || direction["to"] != "true" {
		t.Errorf("the direction was recorded as %v", entry.Changes["offered_here"])
	}
	if _, held := entry.Changes["scope"]; !held {
		t.Error("the entry does not say which level the row belongs to")
	}
}

// Through the registry rather than the handler, because the registry is what a request meets: a
// descriptor that declares an input key the invocation does not use, or the other way round, is a
// use case that answers 422 to every caller and passes every direct test.
func TestTheSwitchIsReachableThroughTheRegistry(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	f.own(t, true)
	id := f.installation(t)

	registry, err := usecase.NewRegistry(nil, f.offer.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), OfferIdentityProviderName, providerActor(), usecase.Input{
		"id":      id.String(),
		"offered": true,
	})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out["offered_here"] != true {
		t.Errorf("the answer says %v", out["offered_here"])
	}
	if _, held := out["client_secret"]; held {
		t.Error("the answer carries the secret")
	}
}
