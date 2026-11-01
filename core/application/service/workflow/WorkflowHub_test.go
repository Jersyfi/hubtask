// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var elsewhere = shared.MustParseID("018f0000-0000-7000-8000-0000000000c5")

// hubDesk is the desk with a second, private hub and a collection in each hub.
func hubDesk() deskFixture {
	f := newDesk()
	deleted := clock{}.Now()
	f.desk.Containers = memoryContainers{rows: map[shared.ID]work.Container{
		hub:          {ID: hub, TenantID: tenant, Type: work.ContainerHub},
		collection:   {ID: collection, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub},
		privateHub:   {ID: privateHub, TenantID: tenant, Type: work.ContainerHub},
		elsewhere:    {ID: elsewhere, TenantID: tenant, Type: work.ContainerCollection, ParentID: privateHub},
		trashedHubID: {ID: trashedHubID, TenantID: tenant, Type: work.ContainerHub, DeletedAt: &deleted},
	}}
	f.desk.Visibility = readable{}
	return f
}

func atHub(path []identity.Scope, at shared.ID) bool {
	return len(path) == 2 && path[1] == identity.HubScope(at)
}

// UC-FLW-02 check 9: a hub's own workflow is shaped with STRUCTURE on that hub, belongs to it, is
// listed only to a caller who may read the hub, and runs only in the hub and its collections.
func TestAHubDefinesItsOwnWorkflow(t *testing.T) {
	ctx := t.Context()
	f := hubDesk()
	create := workflow.CreateWorkflow{Desk: f.desk}

	created, err := create.ExecuteAt(ctx, actor(), hub, "incidents", "Incidents", serviceShape(true))
	if err != nil {
		t.Fatalf("creating at the hub: %v", err)
	}
	if created.HubID != hub || !atHub(f.auth.request.Path, hub) {
		t.Errorf("created %q, authorised along %v", created.HubID, f.auth.request.Path)
	}
	if f.log.changes[len(f.log.changes)-1].ContainerID != hub {
		t.Errorf("the change names no hub: %+v", f.log.changes[len(f.log.changes)-1])
	}
	for name, at := range map[string]shared.ID{"a collection": collection, "a trashed hub": trashedHubID, "no container": entry} {
		if _, err := create.ExecuteAt(ctx, actor(), at, "nowhere", "Nowhere", serviceShape(true)); detail(err) != "workflows.hub_unknown" {
			t.Errorf("%s: %v, want workflows.hub_unknown", name, err)
		}
	}
	if _, err := create.ExecuteAt(ctx, actor(), privateHub, "secret", "Secret", serviceShape(true)); err != nil {
		t.Fatal(err)
	}

	set, _, err := workflow.ListWorkflows{Desk: f.desk}.Execute(ctx, actor())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := set.Find("incidents"); !found {
		t.Error("the readable hub's workflow is not listed")
	}
	if _, found := set.Find("secret"); found {
		t.Error("the private hub's workflow is listed")
	}

	// A draft, a publish and a deletion of the hub's workflow ask STRUCTURE on the hub.
	shape := serviceShape(false)
	if _, err := (workflow.SaveWorkflowDraft{Desk: f.desk}).Execute(ctx, actor(), workflow.SaveWorkflowDraftCommand{Key: "incidents", Shape: &shape}); err != nil {
		t.Fatal(err)
	}
	if !atHub(f.auth.request.Path, hub) {
		t.Errorf("the draft was authorised along %v", f.auth.request.Path)
	}
	if _, _, err := (workflow.PublishWorkflow{Desk: f.desk}).Execute(ctx, actor(), "incidents", nil, 0); err != nil {
		t.Fatal(err)
	}
	if !atHub(f.auth.request.Path, hub) {
		t.Errorf("the publish was authorised along %v", f.auth.request.Path)
	}

	// The scheme: the hub and its collections may run it, another hub's collection may not.
	scheme := workflow.SetWorkflowScheme{Desk: f.desk}
	if _, err := scheme.Execute(ctx, actor(), workflow.SchemeChange{ContainerID: elsewhere, WorkflowKey: "incidents"}, true); detail(err) != "workflows.hub_bound" {
		t.Errorf("another hub's collection ran it: %v", err)
	}
	if _, err := scheme.Execute(ctx, actor(), workflow.SchemeChange{ContainerID: collection, WorkflowKey: "incidents"}, true); err != nil {
		t.Errorf("the hub's collection may run it: %v", err)
	}
}

// UC-FLW-02 check 9: a shared workflow is read-only at a hub; Make a copy here gives the hub its
// own under a new key, the shape in force, naming its source. Another hub's workflow is no source.
func TestAHubCopiesASharedWorkflowHere(t *testing.T) {
	ctx := t.Context()
	f := hubDesk()
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).Execute(ctx, actor(), "service", "Service", serviceShape(true)); err != nil {
		t.Fatal(err)
	}
	if _, err := (workflow.CreateWorkflow{Desk: f.desk}).ExecuteAt(ctx, actor(), privateHub, "secret", "Secret", serviceShape(true)); err != nil {
		t.Fatal(err)
	}
	copyHere := workflow.CopyWorkflowHere{Desk: f.desk}
	copied, err := copyHere.Execute(ctx, actor(), workflow.CopyWorkflowHereCommand{SourceKey: "service", HubID: hub, Key: "support_service"})
	if err != nil {
		t.Fatalf("copying: %v", err)
	}
	if copied.HubID != hub || copied.CopiedFrom != "service" || copied.Name != "Service" || !atHub(f.auth.request.Path, hub) {
		t.Errorf("copy %+v, authorised along %v", copied, f.auth.request.Path)
	}
	if out := workflow.WorkflowOutput(copied); out["hub_id"] != hub.String() || out["copied_from"] != "service" {
		t.Errorf("output %v", out)
	}
	if out := workflow.WorkflowOutput(work.SystemWorkflow()); out["hub_id"] != nil || out["copied_from"] != nil {
		t.Errorf("a workspace's workflow answers a hub: %v", out)
	}
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyWorkflowHereCommand{SourceKey: "secret", HubID: hub, Key: "taken_over"}); detail(err) != "workflows.not_found" {
		t.Errorf("another hub's workflow was copied: %v", err)
	}
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyWorkflowHereCommand{SourceKey: "service", HubID: collection, Key: "in_a_list"}); detail(err) != "workflows.hub_unknown" {
		t.Errorf("a copy at a collection: %v", err)
	}
}
