// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// containerPath is the authorisation path to a container, running from the tenant downwards: the hub it sits
// in, then the container itself.
//
// A membership held at any scope on the path counts, which is what "the effective permission is the highest
// role along the path" means (domain-model.md §3.2). A path that stopped at the collection would refuse
// somebody who holds the right at the hub, and one that stopped at the hub would refuse somebody whose
// membership is on the collection.
//
// A hub's own scope is a hub scope and a collection's is a collection scope. Getting that wrong is not visible
// in a passing test of the common case - a tenant-wide membership authorises everything either way - and only
// a member scoped to exactly one container notices.
func containerPath(container domain.Container) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !container.ParentID.IsZero() {
		path = append(path, identity.HubScope(container.ParentID))
	}
	if container.Type == domain.ContainerHub {
		return append(path, identity.HubScope(container.ID))
	}
	return append(path, identity.CollectionScope(container.ID))
}

// idOrNil is how an optional identifier reaches a projection or an audit entry: the canonical spelling, or an
// explicit null. Nothing here decides that an absent identifier means anything - a null parent says "at the
// top level" by being null, and leaving the field out would say "this server does not know about parents".
func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}
