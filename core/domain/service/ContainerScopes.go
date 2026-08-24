// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// ContainerScopes is the path a container's permissions are resolved along: the tenant, the hub it
// sits under where there is one, and the container itself (domain-model.md §3.2).
//
// Here rather than in an application package because it is a rule about what a container *is*
// rather than about what a use case does with it - and because it now has two readers. The write
// path has resolved it since B-04 and the change stream resolves it per record (C-10); two copies
// of a permission path is the kind of duplicate that stays right until the day a third scope level
// arrives, and then is wrong in exactly one of them.
func ContainerScopes(container work.Container) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !container.ParentID.IsZero() {
		path = append(path, identity.HubScope(container.ParentID))
	}
	if container.Type == work.ContainerHub {
		return append(path, identity.HubScope(container.ID))
	}
	return append(path, identity.CollectionScope(container.ID))
}
