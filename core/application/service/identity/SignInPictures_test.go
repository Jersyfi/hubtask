// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Two workspaces of one installation, each at its own host, as row level security binds every read
// to the transaction's workspace.

var (
	choir      = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	bakery     = shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")
	choirLogo  = shared.ID("01936f2a-7c1e-7000-8000-0000000000e3")
	choirDark  = shared.ID("01936f2a-7c1e-7000-8000-0000000000e4")
	choirMark  = shared.ID("01936f2a-7c1e-7000-8000-0000000000e5")
	bakeryLogo = shared.ID("01936f2a-7c1e-7000-8000-0000000000e6")
)

type scopeKey struct{}

// scopedWork hands each transaction's workspace to the reads inside it.
type scopedWork struct{}

func (scopedWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, scopeKey{}, scope.TenantID))
}

func (scopedWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, scopeKey{}, scope.TenantID))
}

func transactionTenant(ctx context.Context) shared.ID {
	id, _ := ctx.Value(scopeKey{}).(shared.ID)
	return id
}

// installationWorkspaces answers the row of the transaction's workspace and nothing else.
type installationWorkspaces struct {
	rows map[shared.ID]domain.Workspace
}

func (w *installationWorkspaces) Find(ctx context.Context) (domain.Workspace, error) {
	row, held := w.rows[transactionTenant(ctx)]
	if !held {
		return domain.Workspace{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return row, nil
}

func (w *installationWorkspaces) Update(context.Context, domain.Workspace, int, time.Time) (bool, error) {
	return false, errors.New("not written here")
}

// installationHosts names a workspace by the first label of its host.
type installationHosts struct{}

func (installationHosts) Resolve(context.Context, string) (shared.ID, error) {
	return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
}

func (installationHosts) ResolveHost(_ context.Context, host string) (shared.ID, error) {
	switch label, _, _ := strings.Cut(host, "."); label {
	case "choir":
		return choir, nil
	case "bakery":
		return bakery, nil
	}
	return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
}

// installationMedia answers an object only inside its own workspace.
type installationMedia struct {
	mediarepo.Objects
	stored map[shared.ID]media.Object
}

func (m *installationMedia) Find(ctx context.Context, id shared.ID) (media.Object, error) {
	object, held := m.stored[id]
	if !held || object.TenantID != transactionTenant(ctx) {
		return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
	}
	return object, nil
}

func pictureOf(tenantID, id shared.ID) media.Object {
	return media.Object{
		ID: id, TenantID: tenantID, Usage: media.UsageWorkspaceLogo, Status: media.StatusReady,
		ContentType: "image/png", ByteSize: 4096,
	}
}

type installation struct {
	rules      GetSignInRules
	pictures   SignInPictures
	workspaces *installationWorkspaces
	instance   *instanceSettings
	media      *installationMedia
}

func newInstallation() *installation {
	workspaces := &installationWorkspaces{rows: map[shared.ID]domain.Workspace{
		choir: {Tenant: domain.Tenant{ID: choir, DisplayName: "Choir", Status: domain.TenantActive},
			Branding: domain.Branding{Header: domain.HeaderLogo, LogoLight: choirLogo, LogoDark: choirDark, Symbol: choirMark}},
		bakery: {Tenant: domain.Tenant{ID: bakery, DisplayName: "Bakery", Status: domain.TenantActive},
			Branding: domain.Branding{Header: domain.HeaderLogo, LogoLight: bakeryLogo}},
	}}
	instance := &instanceSettings{level: repository.InstanceLevel{
		Policy: domain.PolicyLayer{Locks: map[domain.PolicySwitch]bool{}},
		Legal:  domain.LegalLayer{Locks: map[domain.LegalLink]bool{}},
	}}
	objects := &installationMedia{stored: map[shared.ID]media.Object{}}
	for tenantID, ids := range map[shared.ID][]shared.ID{choir: {choirLogo, choirDark, choirMark}, bakery: {bakeryLogo}} {
		for _, id := range ids {
			objects.stored[id] = pictureOf(tenantID, id)
		}
	}
	rules := GetSignInRules{
		Resolver:   SignInPolicyResolver{Workspaces: workspaces, Instance: instance, UnitOfWork: scopedWork{}},
		Tenants:    installationHosts{},
		Providers:  rulesProviders(),
		UnitOfWork: scopedWork{},
		Clock:      clock.Fixed(time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)),
		Multi:      true, InstallationHost: "tasks.example.org",
	}
	return &installation{
		rules: rules, pictures: SignInPictures{Rules: rules, Objects: objects},
		workspaces: workspaces, instance: instance, media: objects,
	}
}

func (i *installation) card(t *testing.T, host, header string) usecase.Output {
	t.Helper()
	rules, err := i.rules.Execute(t.Context(), GetSignInRulesCommand{Host: host, TenantHeader: header})
	if err != nil {
		t.Fatalf("the rules at %s were refused: %v", host, err)
	}
	return rulesOutput(rules)
}

func picturesOf(out usecase.Output) usecase.Output {
	pictures, _ := out["workspace_pictures"].(usecase.Output)
	return pictures
}

// UC-INS-22 check 8: the rules at a workspace's address carry its choice and the pictures that choice
// draws, each workspace its own; the installation's own address - header or not - and an address no
// workspace answers carry neither, as a workspace that shows the product's name does.
func TestTheRulesAtAWorkspacesAddressCarryItsOwnLookAndNoOther(t *testing.T) {
	i := newInstallation()

	choirCard := i.card(t, "choir.tasks.example.org", "")
	if choirCard["workspace_header"] != "LOGO" {
		t.Fatalf("the choir's card shows %v", choirCard["workspace_header"])
	}
	if got := picturesOf(choirCard); got["logo_light_media_id"] != choirLogo.String() ||
		got["logo_dark_media_id"] != choirDark.String() || got["symbol_media_id"] != nil {
		t.Errorf("the choir's card draws %v, want its two logos and not the symbol beside them", got)
	}
	bakeryCard := i.card(t, "bakery.tasks.example.org", "")
	if got := picturesOf(bakeryCard); got["logo_light_media_id"] != bakeryLogo.String() || len(got) != 1 {
		t.Errorf("the bakery's card draws %v, want its own logo alone", got)
	}

	for _, asked := range []struct{ host, header string }{
		{"tasks.example.org", ""},
		{"tasks.example.org", choir.String()},
		{"nobody.tasks.example.org", ""},
	} {
		out := i.card(t, asked.host, asked.header)
		if _, shown := out["workspace_header"]; shown {
			t.Errorf("%s (header %q) carries a choice: %v", asked.host, asked.header, out["workspace_header"])
		}
		if _, drawn := out["workspace_pictures"]; drawn {
			t.Errorf("%s (header %q) carries pictures", asked.host, asked.header)
		}
	}

	// The product's name chosen, and the switch off above: no choice and no picture either.
	choirRow := i.workspaces.rows[choir]
	choirRow.Branding.Header = domain.HeaderProduct
	i.workspaces.rows[choir] = choirRow
	if out := i.card(t, "choir.tasks.example.org", ""); out["workspace_header"] != nil || out["workspace_pictures"] != nil {
		t.Errorf("the product's name carries %v and %v", out["workspace_header"], out["workspace_pictures"])
	}
	i.instance.level.Policy.Patch = domain.PolicyPatch{OwnBranding: boolOf(false)}
	if out := i.card(t, "bakery.tasks.example.org", ""); out["workspace_header"] != nil || out["workspace_pictures"] != nil {
		t.Errorf("switched off above, the card carries %v and %v", out["workspace_header"], out["workspace_pictures"])
	}
}

// T-34, "never a header", whatever the configuration: an installation that did not state its own
// address treats every host no workspace serves as its own, where the header names the workspace for
// signing in - but the card there still shows the product's mark, and the address that does name a
// workspace still shows that workspace's look.
func TestWithoutAnInstallationAddressAHeaderStillNamesNoLook(t *testing.T) {
	i := newInstallation()
	i.rules.InstallationHost = ""
	i.pictures.Rules.InstallationHost = ""

	for _, header := range []string{choir.String(), bakery.String()} {
		out := i.card(t, "anything.example.net", header)
		if out["workspace_header"] != nil || out["workspace_pictures"] != nil {
			t.Errorf("an unserved host with the header %s carries %v and %v",
				header, out["workspace_header"], out["workspace_pictures"])
		}
	}
	if _, err := i.pictures.Picture(t.Context(), "anything.example.net", choirLogo); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the picture at an unserved host answered %v, want the one 404", err)
	}
	if out := i.card(t, "choir.tasks.example.org", ""); out["workspace_header"] != "LOGO" {
		t.Errorf("the choir's own address shows %v", out["workspace_header"])
	}
}

// The workspace's name draws its symbol beside it, and the answer reaches the contract's document.
func TestTheWorkspacesNameDrawsItsSymbolOnTheCard(t *testing.T) {
	i := newInstallation()
	choirRow := i.workspaces.rows[choir]
	choirRow.Branding.Header = domain.HeaderWorkspaceName
	i.workspaces.rows[choir] = choirRow

	out := i.card(t, "choir.tasks.example.org", "")
	if out["workspace_header"] != "WORKSPACE_NAME" || out["workspace_name"] != "Choir" {
		t.Fatalf("the card shows %v %v", out["workspace_header"], out["workspace_name"])
	}
	if got := picturesOf(out); got["symbol_media_id"] != choirMark.String() || len(got) != 1 {
		t.Errorf("the card draws %v, want the symbol alone", got)
	}
}

// UC-INS-22 check 8, T-34: the public read serves a picture only at the address of its own workspace
// and only while that card draws it; another workspace's picture, a picture the card does not draw,
// one replaced, the installation's own address and a workspace switched off above are one 404.
func TestThePublicReadServesOnlyWhatTheCardAtThatAddressDraws(t *testing.T) {
	i := newInstallation()
	ctx := t.Context()

	if tenantID, err := i.pictures.Picture(ctx, "choir.tasks.example.org", choirLogo); err != nil || tenantID != choir {
		t.Fatalf("the choir's logo at its address answered %q, %v", tenantID, err)
	}
	if tenantID, err := i.pictures.Picture(ctx, "bakery.tasks.example.org:443", bakeryLogo); err != nil || tenantID != bakery {
		t.Fatalf("the bakery's logo at its address answered %q, %v", tenantID, err)
	}

	refused := func(name, host string, id shared.ID) {
		t.Helper()
		_, err := i.pictures.Picture(ctx, host, id)
		if !errors.Is(err, shared.ErrNotFound) || shared.AsError(err).DetailCode != "media.not_found" ||
			len(shared.AsError(err).Params) != 0 {
			t.Errorf("%s answered %v, want the one 404", name, err)
		}
	}
	refused("another workspace's logo at the choir's address", "choir.tasks.example.org", bakeryLogo)
	refused("the choir's logo at the bakery's address", "bakery.tasks.example.org", choirLogo)
	refused("the symbol the card does not draw beside two logos", "choir.tasks.example.org", choirMark)
	refused("the installation's own address", "tasks.example.org", choirLogo)
	refused("an address no workspace answers", "nobody.tasks.example.org", choirLogo)
	refused("the zero identifier", "choir.tasks.example.org", "")

	// Replaced: the old picture is no longer the workspace's.
	choirRow := i.workspaces.rows[choir]
	choirRow.Branding.LogoLight = choirMark
	i.workspaces.rows[choir] = choirRow
	refused("a replaced logo", "choir.tasks.example.org", choirLogo)

	// A picture that is no raster image any more, or was marked for removal, is not served.
	marked := pictureOf(choir, choirDark)
	at := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	marked.DeletedAt = &at
	i.media.stored[choirDark] = marked
	refused("a picture marked for removal", "choir.tasks.example.org", choirDark)
	for _, family := range []string{"image/svg+xml", "video/mp4", "audio/mpeg", "application/pdf", "text/html"} {
		stored := pictureOf(choir, choirDark)
		stored.ContentType = family
		i.media.stored[choirDark] = stored
		refused("a picture stored as "+family, "choir.tasks.example.org", choirDark)
	}

	i.instance.level.Policy.Patch = domain.PolicyPatch{OwnBranding: boolOf(false)}
	refused("the bakery's logo switched off above", "bakery.tasks.example.org", bakeryLogo)
}
