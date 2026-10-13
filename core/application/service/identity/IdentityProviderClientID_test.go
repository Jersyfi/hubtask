// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-INS-11 check 2: a workspace sees the installation's provider by name, kind and admission rule,
// never its secret or its client ID - the installation's registration with the provider is the
// operator's. Through the registry, because that is what REST, MCP and automation all answer from;
// the workspace's own provider still answers its own client ID, which its form edits.
func TestAWorkspaceNeverReadsTheInstallationsClientID(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	own := f.own(t, true)
	installation := f.installation(t)
	writer := f.writer
	writer.Workspaces = f.workspaces

	registry, err := usecase.NewRegistry(nil, ListIdentityProviders{Writer: writer}.Descriptor(), f.offer.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	listed, err := registry.Invoke(t.Context(), ListIdentityProvidersName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	seen := map[string]usecase.Output{}
	for _, row := range rows {
		seen[row["id"].(string)] = row
	}
	if row := seen[installation.String()]; row == nil {
		t.Fatal("the installation's provider is not listed")
	} else {
		if row["client_id"] != "" {
			t.Errorf("a workspace reads the installation's client ID %q", row["client_id"])
		}
		for _, key := range []string{"display_name", "kind", "provisioning"} {
			if row[key] == nil || row[key] == "" {
				t.Errorf("the installation's provider is listed without its %s", key)
			}
		}
		if _, held := row["client_secret"]; held {
			t.Error("the listing carries a secret")
		}
	}
	if row := seen[own.String()]; row == nil || row["client_id"] != "hubtask" {
		t.Errorf("the workspace's own provider does not answer its own client ID: %v", row)
	}

	// The switch answers the row it switched, and the same rule holds there.
	switched, err := registry.Invoke(t.Context(), OfferIdentityProviderName, providerActor(), usecase.Input{
		"id": installation.String(), "offered": true,
	})
	if err != nil {
		t.Fatalf("switching: %v", err)
	}
	if switched["client_id"] != "" {
		t.Errorf("switching answers the installation's client ID %q", switched["client_id"])
	}
}
