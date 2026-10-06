// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The singular surface, kept (SI-10, the concept's first principle: "Keine Funktion verschwindet").
//
// The two things worth asserting are the two a caller written before SI-10 depends on: that the
// route still answers, and that what it answers is a row that caller may write. An installation's
// provider is neither, which is why the skip is a test rather than a comment.

// The route predates the level above the workspace, and must not start handing out its rows: a
// caller that has never heard of an installation provider cannot write one, and would meet a
// refusal on the PUT it makes next.
func TestTheSingularReadSkipsTheInstallationsRows(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	// The installation's row is inserted first, so "the first one listed" is the wrong answer and
	// "the first own one" is the right one. Order is the whole of the test.
	f.installation(t)
	id := f.own(t, true)

	first, err := ReadIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if first.ID != id {
		t.Errorf("the singular route answered %s", first.ID)
	}
	if !first.OfferedHere {
		t.Error("an enabled row of the workspace's own was answered as not a way in here")
	}
}

// With nothing of its own, the route says so rather than answering somebody else's row.
func TestTheSingularReadIsNotFoundWhereOnlyTheInstallationOffersOne(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	f.installation(t)

	_, err := ReadIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor())
	if err == nil {
		t.Fatal("the installation's row was answered as the workspace's own")
	}
	if code := shared.AsError(err).DetailCode; code != "identity_provider.not_configured" {
		t.Errorf("refused with %q", code)
	}
}

// The write has no identifier, so which of two operations it is, is the use case's to decide: with
// nothing configured it adds, and with something configured it replaces *that* one.
func TestTheSingularWriteAddsThenReplacesTheSameRow(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	write := ConfigureFirstIdentityProvider{Writer: f.writer}

	added, err := write.Execute(t.Context(), providerActor(), configureCommand())
	if err != nil {
		t.Fatalf("adding: %v", err)
	}
	if len(f.store.rows) != 1 {
		t.Fatalf("the store holds %d rows after the first write", len(f.store.rows))
	}

	renamed := configureCommand()
	renamed.DisplayName = "Renamed"
	// No secret: the contract's one exception, and the reason a name can be changed without
	// somebody having to find the secret again.
	renamed.ClientSecret = secret.Secret{}
	replaced, err := write.Execute(t.Context(), providerActor(), renamed)
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}
	if replaced.ID != added.ID {
		t.Error("the second write added a second row instead of replacing the first")
	}
	if len(f.store.rows) != 1 {
		t.Errorf("the store holds %d rows, want one", len(f.store.rows))
	}
}

// An installation's row is not what the singular write replaces either: it adds the workspace's
// own beside it, because the row above is not this workspace's to change.
func TestTheSingularWriteDoesNotReplaceTheInstallationsRow(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	above := f.installation(t)

	added, err := ConfigureFirstIdentityProvider{Writer: f.writer}.
		Execute(t.Context(), providerActor(), configureCommand())
	if err != nil {
		t.Fatalf("adding: %v", err)
	}
	if added.ID == above {
		t.Fatal("the singular write replaced the installation's row")
	}
	if added.TenantID != tenant {
		t.Error("the row it added does not belong to this workspace")
	}
	if len(f.store.rows) != 2 {
		t.Errorf("the store holds %d rows, want the installation's and this one", len(f.store.rows))
	}
}

// Both through the registry, because the registry is what a request meets: a descriptor whose
// declared keys and whose invocation disagree answers 422 to every caller and passes every direct
// test (issue 432).
func TestTheSingularSurfaceIsReachableThroughTheRegistry(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))

	registry, err := usecase.NewRegistry(nil,
		ReadIdentityProvider{Writer: f.writer}.Descriptor(),
		ConfigureFirstIdentityProvider{Writer: f.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	written, err := registry.Invoke(
		t.Context(), ConfigureFirstIdentityProviderName, providerActor(), usecase.Input{
			"issuer":                "https://login.example.org",
			"client_id":             "hubtask",
			"client_secret":         "s3cr3t",
			"display_name":          "Example",
			"allowed_email_domains": []any{"example.org"},
			"provisioning":          string(domain.ProvisionDomains),
			"enabled":               false, // echoed by an older client: created off, so accepted
		})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if _, held := written["client_secret"]; held {
		t.Error("the answer carries the secret")
	}

	read, err := registry.Invoke(t.Context(), ReadIdentityProviderName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if read["id"] != written["id"] {
		t.Error("the read answered a different row from the one just written")
	}
	// The route the concept kept has no removal, and the registry is where that is visible: a
	// `DELETE` on a route that cannot say *which* eventually removes the wrong one.
	if _, held := registry.Lookup("RemoveFirstIdentityProvider"); held {
		t.Error("the singular surface grew a removal")
	}
}
