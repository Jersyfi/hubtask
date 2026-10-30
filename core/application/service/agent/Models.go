// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	RegisterAgentModelName = "RegisterAgentModel"
	ListAgentModelsName    = "ListAgentModels"
	ChangeAgentModelName   = "ChangeAgentModel"
)

func modelFields() []usecase.Field {
	return []usecase.Field{
		{Name: "name", Kind: usecase.KindString, Required: true, Description: "What the registry shows."},
		{Name: "source", Kind: usecase.KindString, Required: true, Enum: []string{"OWN", "OFFERED"}, Description: "The workspace's own provider, or a model the installation offers."},
		{Name: "offered_id", Kind: usecase.KindID, Description: "The installation's offered model, for OFFERED."},
		{Name: "model", Kind: usecase.KindString, Required: true, Description: "The model's name at its provider."},
		{Name: "region", Kind: usecase.KindString, Required: true, Enum: []string{"ANY", "ADEQUACY", "EEA", "SELF_HOSTED"}, Description: "Where the model is hosted; at least the installation's minimum jurisdiction."},
		{Name: "retention_days", Kind: usecase.KindInt, Description: "How long the provider keeps what it is sent; omitted is not known, 0 is not at all."},
	}
}

func modelFrom(in usecase.Input) (domain.Model, error) {
	offered, err := optionalID(in, "offered_id")
	if err != nil {
		return domain.Model{}, err
	}
	m := domain.Model{
		Key: in.String("key"), Name: in.String("name"), Source: domain.Source(in.String("source")),
		OfferedID: offered, Model: in.String("model"), Region: domain.Region(in.String("region")),
	}
	if in.Present("retention_days") {
		days := in.Int("retention_days")
		m.RetentionDays = &days
	}
	return m, nil
}

func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

// allowed holds a model to what the workspace's AI allows now, read as the registering person: a
// source the rule allows, a region no weaker than the minimum, an offered model the installation
// still offers. The registration is the consent to send the agents' material there (G9).
func (d Desk) allowed(ctx context.Context, actor appshared.ActorContext, m domain.Model) error {
	if d.Sources == nil {
		return nil
	}
	answer, err := d.Sources.Invoke(ctx, actor, usecase.Input{})
	if err != nil {
		return err
	}
	sources := answer.String("sources")
	switch {
	case sources == "NONE",
		m.Source == domain.SourceOwn && sources == "OFFERED",
		m.Source == domain.SourceOffered && sources == "OWN":
		return shared.ErrForbidden.WithDetail("ai.source_not_allowed").
			WithParams(map[string]string{"source": string(m.Source)})
	}
	if min := domain.Region(answer.String("min_jurisdiction")); min != "" && !m.Region.AtLeast(min) {
		return shared.ErrValidation.WithDetail("agent_models.region_below_minimum").
			WithFields(shared.FieldError{Path: "/region", Code: "agent_models.region_below_minimum",
				Params: map[string]string{"minimum": string(min)}})
	}
	if m.Source == domain.SourceOffered {
		offered, _ := answer["offered"].([]usecase.Output)
		for _, o := range offered {
			if o.String("id") == m.OfferedID.String() {
				return nil
			}
		}
		return shared.ErrValidation.WithDetail("agent_models.offered_unknown").
			WithFields(shared.FieldError{Path: "/offered_id", Code: "agent_models.offered_unknown"})
	}
	return nil
}

// RegisterAgentModel registers a model for the workspace's agents.
type RegisterAgentModel struct{ Desk Desk }

func (h RegisterAgentModel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RegisterAgentModelName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Registers a model the workspace's agents may call: its source (the workspace's own " +
			"provider or a model the installation offers), its name there, where it is hosted and how " +
			"long the provider keeps what it is sent. Registering it is the consent to send agents' " +
			"material there, recorded with who gave it; a source the workspace's AI does not allow or a " +
			"region below the minimum is refused. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the model and an audit entry.",
		TokenScope:  scopeWrite,
		Input: append([]usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "A stable key: lower case letters, digits, - and _."},
		}, modelFields()...),
		Audit:    usecase.AuditDeclaration{Action: ModelRegisteredAction, TargetType: modelTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a model is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			if err := d.administer(ctx, actor, ModelRegisteredAction, scopeWrite, modelTarget, ""); err != nil {
				return nil, err
			}
			m, err := modelFrom(in)
			if err != nil {
				return nil, err
			}
			m, err = domain.CheckModel(m, d.Text)
			if err != nil {
				return nil, err
			}
			if err := d.allowed(ctx, actor, m); err != nil {
				return nil, err
			}
			now := d.Clock.Now()
			m.ID, m.TenantID, m.ConsentBy, m.ConsentAt, m.Revision, m.Version, m.UpdatedAt = d.IDs.NewID(), actor.TenantID, actor.AccountID, now, 1, 1, now
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := d.Store.InsertModel(ctx, m, now); err != nil {
					return err
				}
				return d.record(ctx, actor, ModelRegisteredAction, modelTarget, m.ID, audit.SeverityNotice,
					open("source", string(m.Source)), open("region", string(m.Region)))
			})
			if err != nil {
				return nil, err
			}
			return ModelOutput(m), nil
		}),
	}
}

// ListAgentModels answers the registry of models.
type ListAgentModels struct{ Desk Desk }

func (h ListAgentModels) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentModelsName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "The models registered for the workspace's agents, by key: source, model, region, the " +
			"provider's retention, who consented and when, the revision and whether it is retired.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: ModelsReadAction, TargetType: modelTarget, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			d := h.Desk
			if err := d.administer(ctx, actor, ModelsReadAction, scopeRead, modelTarget, ""); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				models, err := d.Store.ListModels(ctx, maxListed)
				for _, m := range models {
					rows = append(rows, ModelOutput(m))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// ChangeAgentModel changes or retires a registered model. A change of anything an approval relied
// on is a new revision: every agent using the model stops until a person approves it again (G9).
type ChangeAgentModel struct{ Desk Desk }

func (h ChangeAgentModel) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ChangeAgentModelName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Changes a registered model, or retires it. A change to its source, model, region or " +
			"retention is a new revision with a new consent: every agent using it is refused until a " +
			"person approves it for the new revision. A retired model stops every agent using it.",
		SideEffects: "Writes the model and an audit entry.",
		TokenScope:  scopeWrite,
		Input: append([]usecase.Field{
			{Name: "model_id", Kind: usecase.KindID, Required: true},
			{Name: "retired", Kind: usecase.KindBool, Description: "Retires the model; agents using it stop."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		}, modelFields()...),
		Audit:    usecase.AuditDeclaration{Action: ModelChangedAction, TargetType: modelTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a model is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("model_id")
			if err != nil {
				return nil, err
			}
			if err := d.administer(ctx, actor, ModelChangedAction, scopeWrite, modelTarget, id); err != nil {
				return nil, err
			}
			next, err := modelFrom(in)
			if err != nil {
				return nil, err
			}
			var out domain.Model
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				m, err := d.Store.FindModel(ctx, id)
				if err != nil {
					return err
				}
				if in.Present("expected_version") && in.Int("expected_version") != m.Version {
					return shared.ErrVersionConflict
				}
				next.Key = m.Key
				next, err = domain.CheckModel(next, d.Text)
				if err != nil {
					return err
				}
				now := d.Clock.Now()
				revised := m.Revised(next, actor.AccountID, now)
				revised.Retired = m.Retired || in.Bool("retired")
				if revised.Revision != m.Revision && !revised.Retired {
					if err := d.allowed(ctx, actor, revised); err != nil {
						return err
					}
				}
				if err := d.Store.UpdateModel(ctx, revised, m.Version, now); err != nil {
					return err
				}
				revised.Version = m.Version + 1
				out = revised
				return d.record(ctx, actor, ModelChangedAction, modelTarget, id, audit.SeverityNotice,
					open("revision", strconv.Itoa(revised.Revision)), open("retired", strconv.FormatBool(revised.Retired)))
			})
			if err != nil {
				return nil, err
			}
			return ModelOutput(out), nil
		}),
	}
}

// ModelOutput is a model as every door answers it.
func ModelOutput(m domain.Model) usecase.Output {
	out := usecase.Output{
		"id": m.ID.String(), "key": m.Key, "name": m.Name, "source": string(m.Source), "offered_id": idOrNil(m.OfferedID),
		"model": m.Model, "region": string(m.Region), "retention_days": nil, "consent_by": idOrNil(m.ConsentBy),
		"consent_at": m.ConsentAt.UTC(), "revision": m.Revision, "retired": m.Retired, "version": m.Version,
	}
	if m.RetentionDays != nil {
		out["retention_days"] = *m.RetentionDays
	}
	return out
}
