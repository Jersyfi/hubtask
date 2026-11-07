// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func (s *snapshotStore) Workflows(_ context.Context, after shared.ID, batch int) ([]work.Workflow, error) {
	return pageAfter(s.workflows, func(w work.Workflow) string { return w.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) LinkTypes(_ context.Context, after shared.ID, batch int) ([]work.LinkType, error) {
	return pageAfter(s.linkTypes, func(l work.LinkType) string { return l.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) EntryLinks(_ context.Context, after shared.ID, batch int) ([]repository.LinkBetween, error) {
	return pageAfter(s.links, func(l repository.LinkBetween) string { return l.Link.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) CustomRoles(_ context.Context, after shared.ID, batch int) ([]identityrepo.CustomRoleRow, error) {
	return pageAfter(s.roles, func(r identityrepo.CustomRoleRow) string { return r.Role.ID.String() }, after.String(), batch), nil
}

// vocabulary adds what S-05, S-07 and S-08 left out of the replica: a workflow, a link type, a
// custom role, and two links - one inside the first collection, one reaching into the second.
func vocabulary(store *snapshotStore) (inside, across shared.ID) {
	itemA, itemB := idAt('a', 1), idAt('a', 2)
	inside, across = idAt('7', 1), idAt('7', 2)
	store.workflows = []work.Workflow{{ID: idAt('4', 1), TenantID: tenant, Key: "review"}}
	store.linkTypes = []work.LinkType{{ID: idAt('5', 1), TenantID: tenant, Key: "tests", Name: "tests"}}
	store.roles = []identityrepo.CustomRoleRow{{
		Role:    identity.CustomRole{ID: idAt('6', 1), TenantID: tenant, Key: "auditor", Name: "Auditor", Permissions: []string{"read"}},
		Version: 1,
	}}
	store.links = []repository.LinkBetween{
		{Link: work.EntryLink{ID: inside, TenantID: tenant, Type: "blocks", SourceID: itemA, TargetID: itemA},
			SourceCollectionID: collectionA, TargetCollectionID: collectionA},
		{Link: work.EntryLink{ID: across, TenantID: tenant, Type: "blocks", SourceID: itemA, TargetID: itemB},
			SourceCollectionID: collectionA, TargetCollectionID: collectionB},
	}
	return inside, across
}

func TestTheWalkCarriesWorkflowsLinkTypesRolesAndLinks(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = true
	vocabulary(store)

	records, _ := wholeWalk(t, pull, 5)
	// The CMDB's catalogue, walked after them, is its own test's.
	records = slices.DeleteFunc(records, func(r Record) bool {
		return r.Entity == "cmdb_catalogue" || r.Entity == "workflow_scheme"
	})

	got := entities(records)
	tail := got[len(got)-5:]
	want := []string{"link_type", "workflow", "custom_role", "entry_link", "entry_link"}
	if !slices.Equal(tail, want) {
		t.Fatalf("the walk ended with %v, want %v", tail, want)
	}
	for _, record := range records[len(records)-2:] {
		if record.ContainerID != collectionA {
			t.Errorf("a link is filed under %s, want its source's collection", record.ContainerID)
		}
		if record.Payload[workservice.LinkTargetCollectionKey] == nil {
			t.Errorf("a link record names no target collection: %+v", record.Payload)
		}
	}
}

func (s *snapshotStore) WorkflowSchemes(context.Context) ([]work.WorkflowScheme, error) {
	return s.schemes, nil
}

// A board is drawn offline from the scheme: the walk carries it once, under the workspace's
// identifier, with the shipped workflow a device holds no row of.
func TestTheWalkCarriesTheWorkflowSchemeOnce(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = true
	store.schemes = []work.WorkflowScheme{{ContainerID: collectionA, WorkflowKey: "support"}}

	records, _ := wholeWalk(t, pull, 3)

	var schemes []Record
	for _, record := range records {
		if record.Entity == "workflow_scheme" {
			schemes = append(schemes, record)
		}
	}
	if len(schemes) != 1 || schemes[0].EntityID != tenant || !schemes[0].ContainerID.IsZero() {
		t.Fatalf("scheme records %+v, want one under the workspace", schemes)
	}
	rows := schemes[0].Payload["scheme"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["workflow"] != "support" || rows[0]["container_id"] != collectionA.String() {
		t.Errorf("the scheme's rows %v", rows)
	}
	if shipped := schemes[0].Payload["shipped"].(usecase.Output); shipped["key"] != work.SystemWorkflowKey {
		t.Errorf("the shipped workflow %v", shipped)
	}
}

func TestALinkIntoACollectionTheReaderCannotSeeIsWithheld(t *testing.T) {
	pull, f, store := walking(t)
	inside, across := vocabulary(store)
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)

	var links []shared.ID
	for _, record := range records {
		if record.Entity == "entry_link" {
			links = append(links, record.EntityID)
		}
	}
	if !slices.Equal(links, []shared.ID{inside}) {
		t.Errorf("links delivered %v, want only %s - never %s, whose target is out of reach", links, inside, across)
	}
}

func TestACustomRoleNeedsTheWorkspaceButAWorkflowDoesNot(t *testing.T) {
	pull, f, store := walking(t)
	vocabulary(store)
	f.auth.workspace = false

	records, _ := wholeWalk(t, pull, 100)

	got := entities(records)
	if slices.Contains(got, "custom_role") {
		t.Errorf("a custom role reached a member who may not read at the workspace")
	}
	if !slices.Contains(got, "workflow") || !slices.Contains(got, "link_type") {
		t.Errorf("the workspace's vocabulary did not reach every member: %v", got)
	}
}

func TestTheDeltaWithholdsALinkWhoseTargetIsOutOfReach(t *testing.T) {
	link := func(seq int64, target shared.ID) repository.Recorded {
		recorded := entry(seq, collectionA)
		recorded.Entity = "entry_link"
		recorded.Payload = workservice.LinkRecord(work.EntryLink{ID: idAt('7', int(seq))}, collectionA, target)
		return recorded
	}
	removal := entry(3, collectionA)
	removal.Entity, removal.Op = "entry_link", repository.Delete
	pull, f := pulling(t, link(1, collectionA), link(2, collectionB), removal)
	f.auth.allowed[collectionB] = false

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for _, record := range page.Records {
		seqs = append(seqs, record.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 3}) {
		t.Errorf("the delta delivered %v, want the link inside and the removal, not the link across", seqs)
	}
}
