// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/realtime"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// Locator says where a board's live document sits for the realtime service (kind `whiteboard`):
// at the board's hub, read with READ and drawn with WRITE_ITEMS, as the board itself (UC-CNV-01).
// A board in the trash has no live document to open.
type Locator struct{ Studio Studio }

// Locate answers the board's place. It runs inside the realtime service's read-only transaction.
func (l Locator) Locate(ctx context.Context, _ appshared.ActorContext, id shared.ID) (realtime.Place, error) {
	board, err := l.Studio.find(ctx, id, false, false)
	if err != nil {
		return realtime.Place{}, err
	}
	return realtime.Place{Path: hubPath(board.HubID), Read: service.PermissionRead, Write: service.PermissionWriteItems}, nil
}
