// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-12 check 9: a hub defines a field of its own, asked at the hub; a collection or a hub
// together is refused, and so is a collection named as the hub.
func TestAHubDefinesItsOwnFieldAskedAtTheHub(t *testing.T) {
	h := newCustomFieldHarness(t)

	defined, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		HubID: hubID, Key: "region", Kind: domain.CustomFieldText,
	})
	if err != nil {
		t.Fatal(err)
	}
	if defined.HubID != hubID || !defined.CollectionID.IsZero() || defined.IsTenantWide() {
		t.Fatalf("the definition is %+v", defined)
	}
	asked := h.authorizer.requests[0].Path
	if len(asked) != 2 || asked[1].Type != identity.ScopeHub {
		t.Errorf("the path asked about is %v", asked)
	}
	if out := customFieldOutput(defined); out["hub_id"] != hubID.String() || out["collection_id"] != nil {
		t.Errorf("the answer is %v", out)
	}
	if got, _ := h.audit.entries[0].Changes["collection_id"].(map[string]any); got["to"] != hubID.String() {
		t.Errorf("the trail names the scope %v", got)
	}

	for name, cmd := range map[string]DefineCustomFieldCommand{
		"fields.scope_ambiguous": {HubID: hubID, CollectionID: collectionID, Key: "both", Kind: domain.CustomFieldText},
		"fields.hub_unknown":     {HubID: collectionID, Key: "coll", Kind: domain.CustomFieldText},
	} {
		if _, err := h.define.Execute(t.Context(), actor(), cmd); shared.AsError(err).DetailCode != name {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// UC-WRK-12 check 9: a workspace-wide field is copied to a hub under its key with its source
// named; a collection's field is not shared and is not found from there.
func TestAHubCopiesASharedFieldHere(t *testing.T) {
	h := newCustomFieldHarness(t)
	workspaceField, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		Key: "priority", Kind: domain.CustomFieldSelect, Options: []string{"high", "low"},
	})
	if err != nil {
		t.Fatal(err)
	}
	own, err := h.define.Execute(t.Context(), actor(), DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "team_only", Kind: domain.CustomFieldText,
	})
	if err != nil {
		t.Fatal(err)
	}
	copier := CopyCustomFieldHere{Define: h.define}
	asked := len(h.authorizer.requests)

	copied, err := copier.Execute(t.Context(), actor(), CopyCustomFieldHereCommand{SourceID: workspaceField.ID, HubID: hubID})
	if err != nil {
		t.Fatal(err)
	}
	if copied.HubID != hubID || copied.CopiedFrom != workspaceField.ID || copied.Key != "priority" || copied.ID == workspaceField.ID {
		t.Fatalf("the copy is %+v", copied)
	}
	if path := h.authorizer.requests[asked].Path; len(path) != 2 || path[1].Type != identity.ScopeHub {
		t.Errorf("the copy was asked along %v", path)
	}
	if out := customFieldOutput(copied); out["copied_from"] != workspaceField.ID.String() {
		t.Errorf("the answer is %v", out)
	}
	if h.fields.stored[workspaceField.ID].Version != workspaceField.Version {
		t.Error("copying changed the source")
	}

	if _, err := copier.Execute(t.Context(), actor(), CopyCustomFieldHereCommand{SourceID: own.ID, HubID: hubID}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("copying a collection's field gave %v", err)
	}
	if _, err := copier.Execute(t.Context(), actor(), CopyCustomFieldHereCommand{SourceID: workspaceField.ID, HubID: collectionID}); shared.AsError(err).DetailCode != "fields.hub_unknown" {
		t.Errorf("copying to a collection gave %v", err)
	}
}

// UC-WRK-12 check 9: a hub's list is its own and the workspace's, asked at the hub.
func TestAHubListsItsOwnFieldsAndTheWorkspaces(t *testing.T) {
	h := newCustomFieldHarness(t)
	for _, cmd := range []DefineCustomFieldCommand{
		{Key: "everywhere", Kind: domain.CustomFieldText},
		{HubID: hubID, Key: "region", Kind: domain.CustomFieldText},
		{CollectionID: collectionID, Key: "aisle", Kind: domain.CustomFieldText},
	} {
		if _, err := h.define.Execute(t.Context(), actor(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := h.list.Execute(t.Context(), actor(), ListCustomFieldsQuery{HubID: hubID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Errorf("the hub lists %+v", listed)
	}
	if path := h.authorizer.requests[len(h.authorizer.requests)-1].Path; len(path) != 2 {
		t.Errorf("the list was asked along %v", path)
	}
	if _, err := h.list.Execute(t.Context(), actor(), ListCustomFieldsQuery{HubID: hubID, CollectionID: collectionID}); shared.AsError(err).DetailCode != "fields.scope_ambiguous" {
		t.Errorf("both scopes gave %v", err)
	}
}
