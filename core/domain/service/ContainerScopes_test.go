// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The path runs from the tenant downwards, because the effective permission is the highest role
// along it (domain-model.md §3.2): a path that stopped at the collection would refuse somebody who
// holds the right at the hub.
func TestTheScopePathRunsFromTheTenantDownwards(t *testing.T) {
	for _, tc := range []struct {
		name      string
		container work.Container
		want      []identity.Scope
	}{
		{
			name:      "a hub at the top level",
			container: work.Container{ID: hub, Type: work.ContainerHub},
			want:      []identity.Scope{identity.TenantScope(), identity.HubScope(hub)},
		},
		{
			name: "a collection in a hub",
			container: work.Container{
				ID: collectionID, Type: work.ContainerCollection, ParentID: hub,
			},
			want: []identity.Scope{
				identity.TenantScope(), identity.HubScope(hub),
				identity.CollectionScope(collectionID),
			},
		},
		{
			name:      "a collection with no hub above it",
			container: work.Container{ID: collectionID, Type: work.ContainerCollection},
			want: []identity.Scope{
				identity.TenantScope(), identity.CollectionScope(collectionID),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := service.ContainerScopes(tc.container)

			if len(path) != len(tc.want) {
				t.Fatalf("path %v, want %v", path, tc.want)
			}
			for i := range path {
				if path[i] != tc.want[i] {
					t.Errorf("scope %d is %v, want %v", i, path[i], tc.want[i])
				}
			}
		})
	}
}

// A hub's own scope is a hub scope and a collection's is a collection scope. Getting that wrong is
// invisible in the common case - a tenant-wide membership authorises everything either way - and
// only a member scoped to exactly one container notices.
func TestAContainerSitsAtItsOwnKindOfScope(t *testing.T) {
	hubPath := service.ContainerScopes(work.Container{ID: hub, Type: work.ContainerHub})
	collection := service.ContainerScopes(
		work.Container{ID: collectionID, Type: work.ContainerCollection})

	if hubPath[len(hubPath)-1] != identity.HubScope(hub) {
		t.Errorf("a hub ends its path at %v", hubPath[len(hubPath)-1])
	}
	if collection[len(collection)-1] != identity.CollectionScope(collectionID) {
		t.Errorf("a collection ends its path at %v", collection[len(collection)-1])
	}
}
