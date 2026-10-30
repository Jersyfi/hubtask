// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/realtime"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// Locator says where a page's live document sits for the realtime service (kind `page`): at the
// page's hub, read with READ and written with WRITE_ITEMS, as the page itself (UC-DOC-02). A page
// in the trash has no live document to open, and a restricted one none for a reader it keeps out.
type Locator struct{ Space Space }

// Locate answers the page's place. It runs inside the realtime service's read-only transaction.
func (l Locator) Locate(ctx context.Context, actor appshared.ActorContext, id shared.ID) (realtime.Place, error) {
	page, err := l.Space.find(ctx, id, false)
	if err != nil {
		return realtime.Place{}, err
	}
	// A restricted page's document is open to the people it admits alone.
	if err := l.Space.admitted(ctx, actor, page.ID); err != nil {
		return realtime.Place{}, err
	}
	return realtime.Place{Path: hubPath(page.HubID), Read: service.PermissionRead, Write: service.PermissionWriteItems}, nil
}
