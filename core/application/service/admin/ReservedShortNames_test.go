// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// assertReserved holds a refusal to `admin.slug_reserved` at the short name's field.
func assertReserved(t *testing.T, err error, slug string) {
	t.Helper()
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "admin.slug_reserved" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/slug" || refusal.Fields[0].Code != "admin.slug_reserved" {
		t.Fatalf("provisioning %q answered %v, want admin.slug_reserved at /slug", slug, err)
	}
}

// UC-INS-07 check 2: a short name the product reserves is refused at the field, through the use
// case and through the registry REST, MCP and `hubctl` reach it by, before anything is written.
func TestProvisioningRefusesTheProductsReservedShortNames(t *testing.T) {
	for _, slug := range []string{"www", "api", "Admin", "apis", "statuses", "media"} {
		t.Run(slug, func(t *testing.T) {
			f := newProvisionFixture()
			command := provisionCommand()
			command.Slug = slug

			_, err := f.handler.Execute(t.Context(), operator(), command)
			assertReserved(t, err, slug)

			registry, err := usecase.NewRegistry(nil, f.handler.Descriptor())
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			_, err = registry.Invoke(t.Context(), ProvisionTenantName, operator(), usecase.Input{
				"slug": slug, "display_name": "Acme GmbH", "owner_email": "eva@acme.example",
			})
			assertReserved(t, err, slug)

			if len(f.work.scopes) != 0 || len(f.tenants.inserted) != 0 {
				t.Error("a refused short name opened a transaction or wrote a workspace")
			}
		})
	}
}

// UC-INS-07 check 2, UC-INS-19 check 2: the operator's own list, written through the screen's and
// `hubctl`'s door (the registry, as REST hands it the decoded body) or through the instance file,
// is stored the same way and refuses the same names - plural and case included - until a later
// write no longer names it.
func TestTheOperatorsReservedShortNamesRefuseUntilTheListNoLongerNamesThem(t *testing.T) {
	for _, door := range []string{"registry", "file"} {
		t.Run(door, func(t *testing.T) {
			writer, store, _ := newInstanceWriter(newRegister(operatorID))
			write := WriteInstanceSettings{Writer: writer}
			registry, err := usecase.NewRegistry(nil, write.Descriptor())
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			save := func(names ...string) {
				t.Helper()
				list := make([]any, 0, len(names))
				for _, name := range names {
					list = append(list, name)
				}
				document := map[string]any{"reserved_short_names": list}
				switch door {
				case "registry":
					var decoded map[string]any
					encoded, _ := json.Marshal(document)
					if err := json.Unmarshal(encoded, &decoded); err != nil {
						t.Fatalf("decoding: %v", err)
					}
					if _, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input(decoded)); err != nil {
						t.Fatalf("saving %v: %v", names, err)
					}
				case "file":
					level, err := InstanceLevelOf(document)
					if err != nil {
						t.Fatalf("reading %v from the file: %v", names, err)
					}
					if _, err := write.ExecuteAsFile(t.Context(), level, "/etc/hubtask/instance.json"); err != nil {
						t.Fatalf("writing %v from the file: %v", names, err)
					}
				}
			}

			save(" Shop", "intranet", "shop")
			if want := []string{"intranet", "shop"}; !slices.Equal(store.level.ReservedShortNames, want) {
				t.Fatalf("stored %v, want %v", store.level.ReservedShortNames, want)
			}

			provision := func(slug string) error {
				f := newProvisionFixture()
				f.handler.Instance = store
				command := provisionCommand()
				command.Slug = slug
				_, err := f.handler.Execute(t.Context(), operator(), command)
				return err
			}
			for _, slug := range []string{"shop", "SHOPS", "intranet"} {
				assertReserved(t, provision(slug), slug)
			}

			save("intranet")
			if err := provision("shop"); err != nil {
				t.Fatalf("a name the list no longer names was refused: %v", err)
			}
			assertReserved(t, provision("intranet"), "intranet")
			// The product's list stands whatever the operator's says.
			assertReserved(t, provision("www"), "www")
		})
	}
}

// An entry no short name could be is refused at the list's field, the same through both doors, and
// nothing is written.
func TestAnInvalidReservedShortNameIsRefusedByEveryDoor(t *testing.T) {
	writer, store, _ := newInstanceWriter(newRegister(operatorID))
	registry, err := usecase.NewRegistry(nil, WriteInstanceSettings{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	document := map[string]any{"reserved_short_names": []any{"shop", "not a label"}}

	_, throughRegistry := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input(document))
	_, throughFile := InstanceLevelOf(document)
	for door, err := range map[string]error{"registry": throughRegistry, "file": throughFile} {
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != "admin.reserved_short_name_invalid" ||
			len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/reserved_short_names" {
			t.Errorf("the %s answered %v, want admin.reserved_short_name_invalid at /reserved_short_names", door, err)
		}
	}
	if len(store.writes) != 0 {
		t.Error("a refused list was written")
	}
}

// Decision 8 of milestone CD: reserving a name a workspace already carries is accepted and leaves
// that workspace as it is - the list refuses new names only, and nothing but provisioning (and the
// change of address) asks it.
func TestReservingANameAWorkspaceCarriesKeepsTheWorkspace(t *testing.T) {
	f := newProvisionFixture()
	command := provisionCommand()
	command.Slug = "shop"
	if _, err := f.handler.Execute(t.Context(), operator(), command); err != nil {
		t.Fatalf("provisioning shop: %v", err)
	}

	writer, store, _ := newInstanceWriter(newRegister(operatorID))
	level := store.level
	level.ReservedShortNames = []string{"shop"}
	if _, err := (WriteInstanceSettings{Writer: writer}).Execute(t.Context(), operator(), level, 0); err != nil {
		t.Fatalf("reserving a carried name was refused: %v", err)
	}
	if len(f.tenants.inserted) != 1 || f.tenants.inserted[0].Slug != "shop" {
		t.Errorf("the workspace is %+v, want shop kept", f.tenants.inserted)
	}
}

// UC-INS-07 check 2: a short name that is no address label, or that another workspace holds, is
// refused at the field with its own code, through the use case and the registry REST and `hubctl`
// reach it by alike; nothing of the workspace is written.
func TestATakenOrInvalidShortNameIsRefusedAtTheField(t *testing.T) {
	for _, door := range []string{"use case", "registry"} {
		t.Run(door, func(t *testing.T) {
			for _, c := range []struct{ name, slug, code string }{
				{name: "no address label", slug: "Bad Slug!", code: "admin.slug_invalid"},
				{name: "another workspace's", slug: "sales", code: "admin.slug_taken"},
			} {
				t.Run(c.name, func(t *testing.T) {
					f := newProvisionFixture()
					f.tenants.slugsTaken = map[string]bool{"sales": true}
					_, err := provisionBy(t, f, door, usecase.Input{
						"slug": c.slug, "display_name": "Sales", "owner_email": "eva@acme.example",
					})
					refusedAt(t, err, "/slug", c.code)
					if len(f.tenants.inserted) != 0 || len(f.hosts.rows) != 0 {
						t.Errorf("a refused short name wrote %+v, %+v", f.tenants.inserted, f.hosts.rows)
					}
				})
			}
		})
	}
}
