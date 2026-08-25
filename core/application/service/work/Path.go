// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
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
//
// The rule itself moved to the domain when the change stream gained a second reader of it (C-10). This stays
// as the name the write path has always called it by.
func containerPath(container domain.Container) []identity.Scope {
	return service.ContainerScopes(container)
}

// noID is the zero identifier, spelled once. A copy clears several fields to it, and a literal
// conversion at each of them would be the same statement written five ways.
const noID = shared.ID("")

// idOrNil is how an optional identifier reaches a projection or an audit entry: the canonical spelling, or an
// explicit null. Nothing here decides that an absent identifier means anything - a null parent says "at the
// top level" by being null, and leaving the field out would say "this server does not know about parents".
func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}
