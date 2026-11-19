// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var _ repository.RequestPaces = ServiceDeskPaces{}

// ServiceDeskPaces reads how a page of requests moves for their requesters, in one query.
type ServiceDeskPaces struct{}

func (ServiceDeskRepository) Paces() ServiceDeskPaces { return ServiceDeskPaces{} }

func (ServiceDeskPaces) PaceOf(ctx context.Context, itemIDs []shared.ID) (map[shared.ID]servicedesk.RequestPace, error) {
	paces := make(map[shared.ID]servicedesk.RequestPace, len(itemIDs))
	if len(itemIDs) == 0 {
		return paces, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(itemIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RequestPaceOf(ctx, keys)
	if err != nil {
		return nil, queryFailed("reading the requests' pace", err)
	}
	for _, row := range rows {
		id, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		paces[id] = servicedesk.RequestPace{LastReplyAt: optionalTime(row.LastReplyAt), NextDueAt: optionalTime(row.NextDueAt)}
	}
	return paces, nil
}
