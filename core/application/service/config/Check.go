// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"encoding/json"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const CheckConfigName = "CheckConfig"

// CheckConfig answers what the configuration in force would make people meet before they meet it
// (configuration/ui.md A5): the findings of domain.CheckDocument over the document every door
// exports, the workspace's and, for a hub, the hub's own.
type CheckConfig struct{ Desk Desk }

func (h CheckConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       CheckConfigName,
		Permission: service.PermissionStructure,
		Summary: "Checks the configuration in force before people meet it: a workflow whose " +
			"entries can never be completed, a status no step leads to, a step only roles that do " +
			"not exist may take, a rule that answers its own change, a hub's scheme naming a " +
			"workflow the workspace does not hold. Each finding names a severity, a message code, " +
			"the key path of the block to change and its parameters.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{hubField()},
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := hubOf(in)
			if err != nil {
				return nil, err
			}
			return h.Desk.checkInForce(ctx, actor, hub)
		}),
	}
}

func (d Desk) checkInForce(ctx context.Context, actor appshared.ActorContext, hub shared.ID) (usecase.Output, error) {
	if err := d.authorizeAt(ctx, actor, hub, ReadAction, scopeRead, true); err != nil {
		return nil, err
	}
	in := domain.Checked{}
	for _, role := range identity.Roles() {
		in.BuiltInRoles = append(in.BuiltInRoles, string(role))
	}
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		workspace, err := d.export(ctx, actor, "")
		if err != nil {
			return err
		}
		if in.Workspace, err = asJSON(workspace.Document); err != nil {
			return err
		}
		if hub.IsZero() {
			return nil
		}
		place, err := d.export(ctx, actor, hub)
		if err != nil {
			return err
		}
		in.Hub, err = asJSON(place.Document)
		return err
	})
	if err != nil {
		return nil, err
	}
	findings := domain.CheckDocument(in)
	rows := make([]usecase.Output, 0, len(findings))
	for _, finding := range findings {
		row := usecase.Output{"severity": string(finding.Severity), "code": finding.Code, "path": finding.Path}
		if len(finding.Params) > 0 {
			row["params"] = finding.Params
		}
		rows = append(rows, row)
	}
	return usecase.Output{"findings": rows}, nil
}

// asJSON reads a document as every door hands it on, so that the check sees one shape of value
// whichever use case's output a block was built from.
func asJSON(document domain.Document) (domain.Document, error) {
	body, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	out := domain.Document{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
