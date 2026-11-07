// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

type scoped struct{ seen []string }

func (s *scoped) Invoke(_ context.Context, _ string, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	s.seen = actor.Scopes
	return usecase.Output{}, nil
}

func (*scoped) Lookup(name string) (usecase.Descriptor, bool) {
	switch name {
	case "CreateEntryType":
		return usecase.Descriptor{Name: name, TokenScope: "types:write"}, true
	case "PreviewWorkflowPublish":
		return usecase.Descriptor{Name: name, TokenScope: "types:write", ReadOnly: true}, true
	}
	return usecase.Descriptor{}, false
}

// UC-AGT-06 checks 1 and 2: `config:apply:<section>` beside `config:apply` applies that section's
// blocks without each block's own scope, and no other section's; `config:plan` reads a plan's
// previews.
func TestASectionsApplyScopeAdmitsItsBlocksOnly(t *testing.T) {
	cat := &scoped{}
	d := Desk{Catalogue: cat}
	agent := appshared.ActorContext{Scopes: []string{scopeApply, scopeApply + ":types"}}

	if got := d.blockActor(agent, operation{UseCase: "CreateEntryType", Path: "types.incident"}); !slices.Contains(got.Scopes, "types:write") {
		t.Fatalf("the section's scope did not admit its block: %v", got.Scopes)
	}
	if got := d.blockActor(agent, operation{UseCase: "CreateEntryType", Path: "roles.x"}); slices.Contains(got.Scopes, "types:write") {
		t.Fatalf("another section's scope admitted the block: %v", got.Scopes)
	}
	alone := appshared.ActorContext{Scopes: []string{scopeApply + ":types"}}
	if got := d.blockActor(alone, operation{UseCase: "CreateEntryType", Path: "types.incident"}); slices.Contains(got.Scopes, "types:write") {
		t.Fatalf("a section's scope without config:apply admitted the block: %v", got.Scopes)
	}

	planner := appshared.ActorContext{Scopes: []string{scopePlan}}
	if _, err := d.preview(context.Background(), "PreviewWorkflowPublish", planner, usecase.Input{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cat.seen, "types:write") {
		t.Fatalf("config:plan did not read the preview: %v", cat.seen)
	}
	if _, err := d.preview(context.Background(), "CreateEntryType", planner, usecase.Input{}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cat.seen, "types:write") {
		t.Fatalf("config:plan admitted a write: %v", cat.seen)
	}
	if !slices.Contains(ApplyScopes(), "config:apply:workflows") {
		t.Fatalf("scopes = %v", ApplyScopes())
	}
}
