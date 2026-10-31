// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package workflow_test

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	supportHub   = shared.MustParseID("018f0000-0000-7000-8000-0000000000c1")
	privateHub   = shared.MustParseID("018f0000-0000-7000-8000-0000000000c2")
	aCollection  = shared.MustParseID("018f0000-0000-7000-8000-0000000000c3")
	trashedHubID = shared.MustParseID("018f0000-0000-7000-8000-0000000000c4")
)

type hubs map[shared.ID]work.Container

func (h hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := h[id]
	if !found {
		return work.Container{}, shared.ErrNotFound
	}
	return container, nil
}

// readable lets the caller read every hub but the private one.
type readable struct{}

func (readable) Permitted(_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		out[i] = path[len(path)-1].ID != privateHub
	}
	return out, nil
}

func hubFixture() fixture {
	f := newFixture()
	deleted := clock{}.Now()
	f.registry.Hubs = hubs{
		supportHub:   {ID: supportHub, Type: work.ContainerHub},
		privateHub:   {ID: privateHub, Type: work.ContainerHub},
		aCollection:  {ID: aCollection, Type: work.ContainerCollection, ParentID: supportHub},
		trashedHubID: {ID: trashedHubID, Type: work.ContainerHub, DeletedAt: &deleted},
	}
	f.registry.Visibility = readable{}
	return f
}

// UC-FLW-01 check 10: a hub's own type is shaped with STRUCTURE on that hub, belongs to it, and is
// listed only to a caller who may read the hub; the hub must be one of the workspace's living hubs.
func TestAHubDefinesItsOwnType(t *testing.T) {
	ctx := context.Background()
	f := hubFixture()
	create := workflow.CreateEntryType{Registry: f.registry}
	base := workflow.CreateEntryTypeCommand{Level: work.ItemTask, Name: "Incident", Icon: "bug", ColorToken: "red"}

	cmd := base
	cmd.Key, cmd.HubID = "incident", supportHub
	created, err := create.Execute(ctx, actor(), cmd)
	if err != nil {
		t.Fatalf("creating at the hub: %v", err)
	}
	if created.HubID != supportHub {
		t.Errorf("the type belongs to %q", created.HubID)
	}
	path := f.auth.request.Path
	if len(path) != 2 || path[1] != identity.HubScope(supportHub) {
		t.Errorf("authorised along %v, want the hub", path)
	}
	if f.log.changes[0].ContainerID != supportHub {
		t.Errorf("the change names container %q, want the hub", f.log.changes[0].ContainerID)
	}

	for name, hub := range map[string]shared.ID{
		"a collection": aCollection, "a trashed hub": trashedHubID,
		"no hub": shared.MustParseID("018f0000-0000-7000-8000-0000000000cf"),
	} {
		cmd := base
		cmd.Key, cmd.HubID = "elsewhere", hub
		if _, err := create.Execute(ctx, actor(), cmd); detail(err) != "entry_types.hub_unknown" {
			t.Errorf("%s: %v, want entry_types.hub_unknown", name, err)
		}
	}

	cmd = base
	cmd.Key, cmd.HubID = "secret", privateHub
	if _, err := create.Execute(ctx, actor(), cmd); err != nil {
		t.Fatal(err)
	}
	listed, err := workflow.ListEntryTypes{Registry: f.registry}.Execute(ctx, actor())
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, entryType := range listed {
		keys[entryType.Key] = true
	}
	if !keys["incident"] || keys["secret"] {
		t.Errorf("listed %v: want the readable hub's type and not the private one's", keys)
	}

	// A change of the hub's type asks STRUCTURE on the hub, not on the workspace.
	name := "Major incident"
	if _, err := (workflow.ChangeEntryType{Registry: f.registry}).Execute(ctx, actor(), workflow.ChangeEntryTypeCommand{
		Key: "incident", Change: work.EntryTypeChange{Name: &name},
	}); err != nil {
		t.Fatal(err)
	}
	if path := f.auth.request.Path; len(path) != 2 || path[1] != identity.HubScope(supportHub) {
		t.Errorf("the change was authorised along %v", path)
	}
}

// UC-FLW-01 check 10: a shared type is copied to a hub under a key of its own, naming its source;
// the source is untouched, and another hub's type cannot be copied.
func TestMakeACopyHere(t *testing.T) {
	ctx := context.Background()
	f := hubFixture()
	create := workflow.CreateEntryType{Registry: f.registry}
	if _, err := create.Execute(ctx, actor(), workflow.CreateEntryTypeCommand{
		Key: "bug", Level: work.ItemTask, Name: "Bug", Icon: "bug", ColorToken: "red",
	}); err != nil {
		t.Fatal(err)
	}
	copyHere := workflow.CopyEntryTypeHere{Registry: f.registry}
	copied, err := copyHere.Execute(ctx, actor(), workflow.CopyEntryTypeHereCommand{
		SourceKey: "bug", HubID: supportHub, Key: "support_bug",
	})
	if err != nil {
		t.Fatalf("copying: %v", err)
	}
	if copied.HubID != supportHub || copied.CopiedFrom != "bug" || copied.Name != "Bug" {
		t.Errorf("the copy is %+v", copied)
	}
	if source := f.types.rows["bug"]; !source.HubID.IsZero() || source.Version != 1 {
		t.Errorf("the source moved: %+v", source)
	}
	if path := f.auth.request.Path; len(path) != 2 || path[1] != identity.HubScope(supportHub) {
		t.Errorf("authorised along %v", path)
	}
	// A system type is copied as well; one nobody named has no words of its own to carry, so
	// its copy is named (rule 8).
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyEntryTypeHereCommand{
		SourceKey: "task", HubID: supportHub, Key: "support_task",
	}); detail(err) != "entry_types.name_empty" {
		t.Errorf("copying an unnamed system type without a name: %v", err)
	}
	named := "Support task"
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyEntryTypeHereCommand{
		SourceKey: "task", HubID: supportHub, Key: "support_task", Name: &named,
	}); err != nil {
		t.Errorf("copying a system type: %v", err)
	}

	if _, err := create.Execute(ctx, actor(), workflow.CreateEntryTypeCommand{
		Key: "secret", Level: work.ItemTask, Name: "Secret", Icon: "bug", ColorToken: "red", HubID: privateHub,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyEntryTypeHereCommand{
		SourceKey: "secret", HubID: supportHub, Key: "leaked",
	}); detail(err) != "entry_types.not_found" {
		t.Errorf("copying another hub's type: %v, want entry_types.not_found", err)
	}

	f.auth.refuse = true
	if _, err := copyHere.Execute(ctx, actor(), workflow.CopyEntryTypeHereCommand{
		SourceKey: "bug", HubID: supportHub, Key: "refused_copy",
	}); err == nil {
		t.Error("a caller without STRUCTURE on the hub made a copy")
	}
	if _, made := f.types.rows["refused_copy"]; made {
		t.Error("the refused copy was written")
	}
}
