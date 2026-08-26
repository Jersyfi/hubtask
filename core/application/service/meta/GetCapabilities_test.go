// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var tenant = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")

type unitOfWork struct {
	scopes    []persistence.Scope
	readWrite bool
}

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.readWrite = true
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

type profiles struct {
	list []work.CapabilityProfile
	err  error
}

func (p profiles) List(context.Context) ([]work.CapabilityProfile, error) { return p.list, p.err }

// The manifest answers with the profiles in force, so the system defaults are not what it reads.
// The method exists because the port declares it - the hierarchy needs it (ADR-0006).
func (p profiles) ListSystem(context.Context) ([]work.CapabilityProfile, error) {
	return p.list, p.err
}

// languages is what the installation can index, as the database answers it (C-08).
type languages struct {
	tags []string
	err  error
}

func (l languages) List(context.Context) ([]string, error) { return l.tags, l.err }

func systemDefaults() []work.CapabilityProfile {
	return []work.CapabilityProfile{
		{
			Type:              work.ItemTask,
			Capabilities:      []work.Capability{work.CapabilityCompletion, work.CapabilityCover},
			AllowedChildTypes: []work.ItemType{work.ItemWorkPackage},
			MaxDepth:          3,
		},
		{
			Type:         work.ItemActivity,
			Capabilities: []work.Capability{work.CapabilityCompletion},
			MaxDepth:     1,
		},
	}
}

func handler(store profiles, uow *unitOfWork) GetCapabilities {
	return GetCapabilities{
		Profiles:   store,
		Languages:  languages{tags: []string{"de", "en"}},
		UnitOfWork: uow,
		Config: env.Config{
			Version: "1.2.3",
			Tenancy: env.TenancySingle,
			Request: env.RequestConfig{MaxBodyBytes: 1 << 20, MaxUploadBytes: 1 << 26},
			Mail:    env.MailConfig{Host: "smtp.example.com"},
		},
	}
}

func TestTheManifestAnswersFromTheDatabase(t *testing.T) {
	uow := &unitOfWork{}

	capabilities, err := handler(profiles{list: systemDefaults()}, uow).
		Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(capabilities.ItemTypes) != 2 {
		t.Fatalf("%d item types", len(capabilities.ItemTypes))
	}
	if capabilities.ItemTypes[0].Type != work.ItemTask || capabilities.ItemTypes[0].MaxDepth != 3 {
		t.Errorf("first profile = %+v", capabilities.ItemTypes[0])
	}
	if !capabilities.ItemTypes[0].Allows(work.CapabilityCover) {
		t.Error("a task cannot have a cover")
	}
	if capabilities.ItemTypes[1].Allows(work.CapabilityCover) {
		t.Error("an activity can have a cover")
	}
}

// An anonymous caller reads the rows that belong to no tenant. With no tenant context set, every
// policy comparing against one is false - which is stricter than a tenant scope, not looser.
func TestAnAnonymousCallerReadsTheInstallationScope(t *testing.T) {
	uow := &unitOfWork{}

	if _, err := handler(profiles{list: systemDefaults()}, uow).
		Execute(t.Context(), appshared.Anonymous("en", "UTC")); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(uow.scopes) != 1 {
		t.Fatalf("%d transactions", len(uow.scopes))
	}
	if !uow.scopes[0].Installation {
		t.Errorf("scope = %+v, want the installation scope", uow.scopes[0])
	}
	if !uow.scopes[0].TenantID.IsZero() {
		t.Error("the anonymous read claimed a tenant")
	}
}

func TestAnAuthenticatedCallerReadsItsOwnTenant(t *testing.T) {
	uow := &unitOfWork{}
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant}

	if _, err := handler(profiles{list: systemDefaults()}, uow).Execute(t.Context(), actor); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if uow.scopes[0].Installation {
		t.Error("an authenticated caller read the installation scope and would miss its overrides")
	}
	if uow.scopes[0].TenantID != tenant {
		t.Errorf("scope = %+v", uow.scopes[0])
	}
}

// The manifest is a read. A read-write transaction would be refused under the installation scope
// anyway, and asking for one here would be the mistake that surfaces as a policy violation.
func TestTheManifestNeverOpensAWriteTransaction(t *testing.T) {
	uow := &unitOfWork{}

	if _, err := handler(profiles{list: systemDefaults()}, uow).
		Execute(t.Context(), appshared.Anonymous("en", "UTC")); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if uow.readWrite {
		t.Error("the manifest opened a read-write transaction")
	}
}

func TestTheManifestReportsTheInstallation(t *testing.T) {
	capabilities, err := handler(profiles{list: systemDefaults()}, &unitOfWork{}).
		Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if capabilities.ProductVersion != "1.2.3" || capabilities.APIVersion != APIVersion {
		t.Errorf("versions = %q / %q", capabilities.ProductVersion, capabilities.APIVersion)
	}
	if capabilities.TenancyMode != string(env.TenancySingle) {
		t.Errorf("tenancy = %q", capabilities.TenancyMode)
	}
	if capabilities.Limits["max_body_bytes"] != 1<<20 {
		t.Errorf("limits = %v", capabilities.Limits)
	}
	// The two product-shaped bounds a client needs before it builds an editor: how many reminders
	// an entry carries, and how large a template's tree may be.
	if capabilities.Limits["max_reminders_per_item"] != int64(work.MaxRemindersPerItem) ||
		capabilities.Limits["max_template_nodes"] != int64(work.MaxTemplateNodes) {
		t.Errorf("the product's own bounds are missing from %v", capabilities.Limits)
	}
	if !capabilities.Features["mail"] {
		t.Error("a configured SMTP server is not reported as a feature")
	}
}

// Nothing is invented when the SMTP server is absent: a client uses this to decide whether to
// offer the action at all.
func TestAnUnconfiguredFeatureIsReportedAsAbsent(t *testing.T) {
	h := handler(profiles{list: systemDefaults()}, &unitOfWork{})
	h.Config.Mail.Host = ""

	capabilities, err := h.Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if capabilities.Features["mail"] {
		t.Error("mail is reported although no server is configured")
	}
}

func TestAReadFailureSurfaces(t *testing.T) {
	_, err := handler(profiles{err: shared.ErrUnavailable}, &unitOfWork{}).
		Execute(t.Context(), appshared.Anonymous("en", "UTC"))

	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error = %v", err)
	}
}

// C-04: the manifest describes the role matrix, and describes it out of the matrix rather than
// from a copy - so that a client renders the actions it offers from data.
func TestTheManifestReportsTheRoleMatrix(t *testing.T) {
	manifest, err := handler(profiles{list: systemDefaults()}, &unitOfWork{}).
		Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}

	if len(manifest.Roles) != len(identity.Roles()) {
		t.Fatalf("%d rows, want one per defined role", len(manifest.Roles))
	}

	described := map[identity.Role]RoleDescription{}
	for _, row := range manifest.Roles {
		described[row.Role] = row
	}

	// Every kind of access is answered for every role: an absent key would leave a client
	// guessing, and the permissive guess is what this endpoint exists to prevent.
	for role, row := range described {
		for _, action := range service.ItemActions() {
			if _, answered := row.ItemAccess[action]; !answered {
				t.Errorf("%s has no answer for %s", role, action)
			}
		}
	}

	// The two cells no permission name carries, which are the reason the section exists.
	contributor := described[identity.RoleContributor]
	if contributor.ItemAccess[service.ItemChange] != service.AccessAssigned {
		t.Errorf("a contributor's change is %q, want ASSIGNED", contributor.ItemAccess[service.ItemChange])
	}
	if contributor.ItemAccess[service.ItemCreate] != service.AccessAll {
		t.Errorf("a contributor's create is %q, want ALL", contributor.ItemAccess[service.ItemCreate])
	}

	guest := described[identity.RoleGuest]
	if guest.ItemAccess[service.ItemComment] != service.AccessAll {
		t.Errorf("a guest's comment is %q, want ALL", guest.ItemAccess[service.ItemComment])
	}
	if guest.ItemAccess[service.ItemChange] != service.AccessNone {
		t.Errorf("a guest's change is %q, want NONE", guest.ItemAccess[service.ItemChange])
	}

	// The permission half comes from the same table: an administrator ranks above a member and
	// still may not delete a container.
	admin := described[identity.RoleAdmin]
	for _, permission := range admin.Permissions {
		if permission == service.PermissionDeleteContainer {
			t.Error("the manifest gives an administrator DELETE_CONTAINER")
		}
	}
}

// The languages this installation can index come from the database, in the same transaction as
// the profiles: both are answers about this installation rather than about the product, and a
// client's language picker is built from the list rather than from a constant (C-08, ADR-0034).
func TestTheManifestAnswersWhichLanguagesCanBeIndexed(t *testing.T) {
	uow := &unitOfWork{}

	capabilities, err := handler(profiles{list: systemDefaults()}, uow).
		Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if len(capabilities.TextLanguages) != 2 || capabilities.TextLanguages[0] != "de" {
		t.Errorf("the languages are %v, want what the database answered", capabilities.TextLanguages)
	}
	// One transaction, not two: the manifest is read before a client has signed in, and a second
	// round trip for it is one every cold start pays.
	if len(uow.scopes) != 1 {
		t.Errorf("%d transactions were opened, want one", len(uow.scopes))
	}
}

// An installation whose PostgreSQL can index nothing beyond exact words says so, and the failure
// to read the list at all is a failure of the manifest rather than a silent empty one: a client
// that saw no languages would offer none.
func TestAnUnreadableLanguageListFailsTheManifest(t *testing.T) {
	handler := handler(profiles{list: systemDefaults()}, &unitOfWork{})
	handler.Languages = languages{err: errors.New("the catalogue is unreadable")}

	if _, err := handler.Execute(t.Context(), appshared.Anonymous("en", "UTC")); err == nil {
		t.Fatal("the manifest was answered without the languages it promises")
	}
}

// The two backup flags (E-03). What is configured rather than what is implemented: a client that
// offers "add an S3 target" on an installation with no encryption keyring is offering a form that
// will be refused at the end.
func TestTheManifestSaysWhetherABackupTargetCanBeConfigured(t *testing.T) {
	answer := func(cfg env.Config) Capabilities {
		t.Helper()
		handler := handler(profiles{list: systemDefaults()}, &unitOfWork{})
		handler.Config = cfg
		capabilities, err := handler.Execute(t.Context(), appshared.Anonymous("en", "UTC"))
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		return capabilities
	}

	bare := answer(env.Config{Tenancy: env.TenancySingle})
	if bare.Features["backup_encryption"] {
		t.Error("an installation with no keyring says it can seal a credential")
	}
	// One tenant means its owner is the operator, so there is nobody the switch could protect.
	if !bare.Features["backup_targets"] {
		t.Error("a single-tenant installation says its tenant may not configure a target")
	}

	provider := answer(env.Config{
		Tenancy: env.TenancyMulti,
		Encryption: env.EncryptionConfig{
			Keys: []env.EncryptionKey{{ID: "k2026", Material: secret.New("material")}},
		},
	})
	if !provider.Features["backup_encryption"] {
		t.Error("an installation with a keyring says it cannot seal a credential")
	}
	if provider.Features["backup_targets"] {
		t.Error("provider operation says a tenant may configure a target with the switch off")
	}

	provider.Features = answer(env.Config{
		Tenancy: env.TenancyMulti,
		Backup:  env.BackupConfig{TenantTargets: true},
	}).Features
	if !provider.Features["backup_targets"] {
		t.Error("the switch is on and the manifest says otherwise")
	}
}
