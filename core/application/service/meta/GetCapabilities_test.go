// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
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
