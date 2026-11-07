// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/realtime"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// PresenceLocator says where an entry's presence sits for the realtime service (kind `entry`):
// at the entry's collection, read with READ on the entry itself, so a role reading only its own
// entries sees the others of those alone. Presence is awareness only - who has the entry open,
// who is writing a reply (collision detection) - and nothing of it is written. A trashed entry
// has none.
type PresenceLocator struct {
	Items      repository.Items
	Containers repository.Containers
}

// Locate answers the entry's place. It runs inside the realtime service's read-only transaction.
func (l PresenceLocator) Locate(ctx context.Context, _ appshared.ActorContext, id shared.ID) (realtime.Place, error) {
	item, err := l.Items.Find(ctx, id)
	if err != nil {
		return realtime.Place{}, err
	}
	if item.IsTrashed() {
		return realtime.Place{}, shared.ErrNotFound.WithDetail("realtime.document_not_found")
	}
	collection, err := l.Containers.Find(ctx, item.CollectionID)
	if err != nil {
		return realtime.Place{}, err
	}
	return realtime.Place{
		Path: containerPath(collection), Read: service.PermissionRead, On: reading(item), PresenceOnly: true,
	}, nil
}
