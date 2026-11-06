// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// assignedFixture is provisioning on an installation at tasks.corp.example that assigns hosts.
func assignedFixture() *provisionFixture {
	f := newProvisionFixture()
	f.handler.WorkspaceHosts = env.HostsAssigned
	f.handler.InstallationHost = "tasks.corp.example"
	f.hosts.taken = map[string]bool{"tasks-sales.corp.example": true}
	return f
}

// provisionBy provisions through the use case itself, or through the registry REST and `hubctl`
// reach it by - the decoded body, as the adapter hands it in.
func provisionBy(t *testing.T, f *provisionFixture, door string, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	if door == "use case" {
		provisioned, err := f.handler.Execute(t.Context(), operator(), ProvisionTenantCommand{
			Slug: in.String("slug"), Host: in.String("host"), DisplayName: in.String("display_name"),
			OwnerEmail: in.String("owner_email"),
		})
		if err != nil {
			return nil, err
		}
		return usecase.Output{"slug": provisioned.Tenant.Slug, "host": provisioned.Host}, nil
	}
	registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry.Invoke(t.Context(), ProvisionTenantName, operator(), in)
}

func refusedAt(t *testing.T, err error, path, code string) {
	t.Helper()
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != code || len(refusal.Fields) != 1 ||
		refusal.Fields[0].Path != path || refusal.Fields[0].Code != code {
		t.Fatalf("answered %v, want %s at %s", err, code, path)
	}
}

// UC-INS-20 check 1: an installation that assigns hosts asks for the workspace's host, through the
// use case and the registry REST and `hubctl` reach it by alike, and no longer for a subdomain: the
// host becomes the workspace's main row, ACTIVE at once - no DNS proof is asked (check 3) - and the
// short name, its identifier for `hubctl` and the API, is made from the host where none is given.
func TestAnInstallationThatAssignsHostsAsksForTheHost(t *testing.T) {
	for _, door := range []string{"use case", "registry"} {
		t.Run(door, func(t *testing.T) {
			for _, c := range []struct {
				name, slug, wantSlug string
			}{
				{name: "the short name made from the host", wantSlug: "tasks-hr"},
				{name: "the short name given", slug: "people", wantSlug: "people"},
			} {
				t.Run(c.name, func(t *testing.T) {
					f := assignedFixture()
					in := usecase.Input{
						"host": "Tasks-HR.Corp.Example", "display_name": "Human Resources",
						"owner_email": "jana.koch@corp.example",
					}
					if c.slug != "" {
						in["slug"] = c.slug
					}
					out, err := provisionBy(t, f, door, in)
					if err != nil {
						t.Fatalf("provisioning: %v", err)
					}
					if out["slug"] != c.wantSlug || out["host"] != "tasks-hr.corp.example" {
						t.Errorf("answered slug %v at %v, want %s at tasks-hr.corp.example", out["slug"], out["host"], c.wantSlug)
					}
					if len(f.hosts.rows) != 1 {
						t.Fatalf("%d host rows written, want the one main row", len(f.hosts.rows))
					}
					row := f.hosts.rows[0]
					if row.Host != "tasks-hr.corp.example" || !row.Canonical || row.State != domain.HostActive {
						t.Errorf("wrote %+v, want the assigned host, canonical and ACTIVE", row)
					}
					if f.tenants.inserted[0].Slug != c.wantSlug {
						t.Errorf("the workspace's short name is %q, want %q", f.tenants.inserted[0].Slug, c.wantSlug)
					}
				})
			}
		})
	}
}

// UC-INS-20 check 3: a host another workspace uses, the installation's own host and what is no host
// name are refused at the field, each with a code naming its reason, by every door alike; a missing
// one is asked for at the field too.
func TestAnAssignedHostIsRefusedAtTheFieldWithItsReason(t *testing.T) {
	for _, door := range []string{"use case", "registry"} {
		t.Run(door, func(t *testing.T) {
			for _, c := range []struct {
				name, host, code string
			}{
				{name: "a host another workspace uses", host: "tasks-sales.corp.example", code: "admin.host_taken"},
				{name: "the installation's own host", host: "tasks.corp.example", code: "admin.host_installation"},
				{name: "no host name", host: "https://tasks hr/", code: "admin.host_invalid"},
				{name: "no host at all", host: "", code: "admin.host_required"},
			} {
				t.Run(c.name, func(t *testing.T) {
					f := assignedFixture()
					_, err := provisionBy(t, f, door, usecase.Input{
						"host": c.host, "display_name": "Sales", "owner_email": "eva@corp.example",
					})
					refusedAt(t, err, "/host", c.code)
					if len(f.hosts.rows) != 0 {
						t.Errorf("a refused host wrote %+v", f.hosts.rows)
					}
					if c.code != "admin.host_taken" && (len(f.work.scopes) != 0 || len(f.tenants.inserted) != 0) {
						t.Error("a host refused before the write opened a transaction or wrote a workspace")
					}
				})
			}
		})
	}
}

// With subdomains nothing about assigned hosts is taken (ADR-0095 §6): a host is
// refused at its field - and the short name stays required, as the subdomain it is.
func TestSubdomainsTakeNoHost(t *testing.T) {
	for _, door := range []string{"use case", "registry"} {
		t.Run(door, func(t *testing.T) {
			f := newProvisionFixture()
			_, err := provisionBy(t, f, door, usecase.Input{
				"slug": "acme", "host": "tasks.acme.example", "display_name": "Acme GmbH",
				"owner_email": "eva@acme.example",
			})
			refusedAt(t, err, "/host", "admin.host_not_assigned")

			_, err = provisionBy(t, f, door, usecase.Input{
				"display_name": "Acme GmbH", "owner_email": "eva@acme.example",
			})
			refusedAt(t, err, "/slug", "admin.slug_required")

			out, err := provisionBy(t, f, door, usecase.Input{
				"slug": "acme", "display_name": "Acme GmbH", "owner_email": "eva@acme.example",
			})
			if err != nil || out["host"] != "acme.hubtask.example" {
				t.Errorf("provisioning by short name answered (%v, %v), want the subdomain", out, err)
			}
		})
	}
}
