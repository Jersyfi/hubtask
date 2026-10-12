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

var (
	newcomerGroup      = shared.ID("01936f2a-7c1e-7000-8000-0000000002b1")
	newcomerOwnerGroup = shared.ID("01936f2a-7c1e-7000-8000-0000000002b2")
)

// What newcomers get is set on the provider's form, through the registry the way a request sets it
// (UC-ID-11 check 5): a role or a group of this workspace, recorded in the trail by code and
// identifier; a group the workspace does not have, or one holding OWNER anywhere, is refused.
func TestWhatNewcomersGetIsSetThroughTheRegistry(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)
	f.writer.Groups = newGroups(
		domain.Group{ID: newcomerGroup, TenantID: tenant, Name: "Staff", Version: 1},
		domain.Group{ID: newcomerOwnerGroup, TenantID: tenant, Name: "Owners", Version: 1},
	)
	f.writer.Grants = newGrants(domain.Grant{
		ID: "01936f2a-7c1e-7000-8000-0000000002b3", TenantID: tenant, GroupID: newcomerOwnerGroup,
		Scope: domain.TenantScope(), Role: domain.RoleOwner,
	})
	registry, err := usecase.NewRegistry(nil, ConfigureIdentityProvider{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	configure := func(extra usecase.Input) (usecase.Output, error) {
		in := usecase.Input{
			"issuer": "https://login.example.org", "client_id": "hubtask", "client_secret": "s3cr3t",
			"provisioning": "ANY",
		}
		for key, value := range extra {
			in[key] = value
		}
		return registry.Invoke(t.Context(), ConfigureIdentityProviderName, providerActor(), in)
	}

	added, err := configure(usecase.Input{"newcomer_role": "MEMBER"})
	if err != nil {
		t.Fatalf("a role for newcomers: %v", err)
	}
	if added["newcomer_role"] != "MEMBER" || added["newcomer_group_id"] != nil {
		t.Errorf("the answer is %v", added)
	}
	changes := f.session.audit.entries[len(f.session.audit.entries)-1].Changes
	if role, _ := changes["newcomer_role"].(map[string]any); role["to"] != "MEMBER" {
		t.Errorf("the trail says %v", changes)
	}

	replaced, err := configure(usecase.Input{"id": added["id"], "newcomer_group_id": string(newcomerGroup)})
	if err != nil {
		t.Fatalf("a group for newcomers: %v", err)
	}
	if replaced["newcomer_group_id"] != string(newcomerGroup) || replaced["newcomer_role"] != nil {
		t.Errorf("the replacement answered %v", replaced)
	}

	// Set whole: a replace that names nothing gives newcomers nothing.
	cleared, err := configure(usecase.Input{"id": added["id"]})
	if err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if cleared["newcomer_group_id"] != nil || cleared["newcomer_role"] != nil {
		t.Errorf("a replace without newcomers kept %v", cleared)
	}

	refusals := []struct {
		name string
		in   usecase.Input
		code string
	}{
		{"a group this workspace does not have",
			usecase.Input{"newcomer_group_id": "01936f2a-7c1e-7000-8000-0000000002ff"}, "groups.not_found"},
		{"a group that holds OWNER", usecase.Input{"newcomer_group_id": string(newcomerOwnerGroup)},
			"identity_provider.newcomers_owner"},
		{"OWNER itself", usecase.Input{"newcomer_role": "OWNER"}, "identity_provider.newcomers_owner"},
		{"under INVITED_ONLY", usecase.Input{"provisioning": "INVITED_ONLY", "newcomer_role": "MEMBER"},
			"identity_provider.newcomers_not_created"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			_, err := configure(tc.in)
			if err == nil || shared.AsError(err).DetailCode != tc.code {
				t.Fatalf("answered %v, want %s", err, tc.code)
			}
		})
	}
}
