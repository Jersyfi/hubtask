// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// CiSummaries answers the configuration items among identifiers (M6).
type CiSummaries interface {
	CIsByID(ctx context.Context, ids []shared.ID) ([]cmdb.CI, error)
}

// visibleCis answers the CIs among ids the reader may read: READ at the CI's hub, the assets
// module's rule.
func (l Linking) visibleCis(ctx context.Context, actor appshared.ActorContext, ids []shared.ID, action audit.Action) (map[shared.ID]cmdb.CI, error) {
	visible := map[shared.ID]cmdb.CI{}
	if len(ids) == 0 || l.Cis == nil {
		return visible, nil
	}
	var cis []cmdb.CI
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := l.Cis.CIsByID(ctx, ids)
		cis = read
		return err
	})
	if err != nil || len(cis) == 0 {
		return visible, err
	}
	paths := make([][]identity.Scope, 0, len(cis))
	for _, ci := range cis {
		paths = append(paths, []identity.Scope{identity.TenantScope(), identity.HubScope(ci.HubID)})
	}
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: "cmdb_ci",
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, ci := range cis {
		if i < len(allowed) && allowed[i] {
			visible[ci.ID] = ci
		}
	}
	return visible, nil
}

// ciTarget is a readable CI as a reference shows it (schema ReferenceTarget): its name, class,
// stage and hub - never its attributes, which a reference does not carry.
func ciTarget(ci cmdb.CI, anchor string) usecase.Output {
	return usecase.Output{
		"kind": string(reference.KindCI), "id": ci.ID.String(), "anchor": anchorOrNil(anchor),
		"available": true, "title": ci.Name, "hub_id": ci.HubID.String(), "ci_class": ci.Class, "stage": string(ci.Stage),
	}
}
