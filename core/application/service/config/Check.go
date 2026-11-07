// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"encoding/json"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workmodel "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const CheckConfigName = "CheckConfig"

// CheckConfig answers what the configuration in force would make people meet before they meet it
// (configuration.md §11): the findings of domain.CheckDocument over the document every door
// exports, the workspace's and, for a hub, the hub's own.
type CheckConfig struct{ Desk Desk }

func (h CheckConfig) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       CheckConfigName,
		Permission: service.PermissionStructure,
		Summary: "Checks the configuration in force before people meet it: a workflow whose " +
			"entries can never be completed, a status no step leads to, a step only roles that do " +
			"not exist may take, a rule that answers its own change, a hub's scheme naming a " +
			"workflow the workspace does not hold, a required field every channel hides, two fields " +
			"of one kind offering the same choices, one tolerance locked alike in several " +
			"collections the hub could lock once. Each finding names a severity, a message code, " +
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
	in := checkedBase()
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

// checkedBase is what every check knows beside the documents: the built-in roles and the doors.
func checkedBase() domain.Checked {
	in := domain.Checked{}
	for _, role := range identity.Roles() {
		in.BuiltInRoles = append(in.BuiltInRoles, string(role))
	}
	for _, channel := range workmodel.FormChannels() {
		in.Channels = append(in.Channels, string(channel))
	}
	return in
}

// checkTemplate refuses a template the check finds an error in (configuration.md §11): one refusal
// per error, at the block it is about, naming the finding - a broken template is never offered.
// Warnings publish; the template is checked alone, as a workspace taking it would hold it.
func checkTemplate(document, hubPart domain.Document) error {
	in := checkedBase()
	var err error
	if in.Workspace, err = asJSON(document); err != nil {
		return err
	}
	if hubPart != nil {
		if in.Hub, err = asJSON(hubPart); err != nil {
			return err
		}
	}
	var refusals []shared.FieldError
	for _, finding := range domain.CheckDocument(in) {
		if finding.Severity != domain.SeverityError {
			continue
		}
		root := "/document/"
		if in.Hub != nil && strings.HasPrefix(finding.Path, "scheme.") {
			root = "/hub_document/"
		}
		refusals = append(refusals, shared.FieldError{
			Path: root + strings.ReplaceAll(finding.Path, ".", "/"), Code: "config.template_check_failed",
			Params: map[string]string{"finding": finding.Code},
		})
	}
	if len(refusals) > 0 {
		return domain.ErrRefused(refusals)
	}
	return nil
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
