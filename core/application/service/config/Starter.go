// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// StarterTemplates takes a template for a workspace's first start (UC-WRK-30 check 5,
// user-experience.md §3.2): the plan a person would get from PlanConfigTemplate, applied as
// ApplyConfig applies it - the same authorisation, hold, refusals and provenance - inside the
// starter's transaction, so a refused part leaves no hub and the question open.
type StarterTemplates struct{ Desk Desk }

var (
	_ work.StarterTemplates = StarterTemplates{}
	_ work.StarterBaseline  = StarterTemplates{}
)

// Baseline records the workspace, and the hub the first start made if any, as the start left them,
// as a version of its own where it has none or moved since (drifted): a start that took no
// template, or was skipped, otherwise has no version to compare a screen's change with or to roll
// back to. Called inside the starter's transaction, which authorised the actor.
func (s StarterTemplates) Baseline(ctx context.Context, actor appshared.ActorContext, hub shared.ID) error {
	d := s.Desk
	if err := d.Store.Hold(ctx); err != nil {
		return err
	}
	scopes := []shared.ID{""}
	if !hub.IsZero() {
		scopes = append(scopes, hub)
	}
	for _, scope := range scopes {
		now, err := d.export(ctx, actor, scope)
		if err != nil {
			return err
		}
		if err := d.drifted(ctx, actor, now, d.Clock.Now()); err != nil {
			return err
		}
	}
	return nil
}

// TakeForWorkspace applies the template's workspace part.
func (s StarterTemplates) TakeForWorkspace(ctx context.Context, actor appshared.ActorContext, take work.StarterTemplate) error {
	return s.take(ctx, actor, take, "")
}

// TakeForHub applies the template's hub part to the hub just made, with its samples if asked. A
// template without a hub part - a register's - leaves the hub as its preset made it.
func (s StarterTemplates) TakeForHub(ctx context.Context, actor appshared.ActorContext, take work.StarterTemplate, hub shared.ID) error {
	forHubs, err := s.forHubs(ctx, actor, take.Key)
	if err != nil || !forHubs {
		return err
	}
	return s.take(ctx, actor, take, hub)
}

func (s StarterTemplates) take(ctx context.Context, actor appshared.ActorContext, take work.StarterTemplate, hub shared.ID) error {
	d := s.Desk
	if err := d.authorizeAt(ctx, actor, hub, AppliedAction, scopeApply, false); err != nil {
		return err
	}
	planned, err := d.planTemplate(ctx, actor, hub, "", take.Key, 0, nil, take.Answers, take.Samples && !hub.IsZero())
	if err != nil {
		return err
	}
	in := usecase.Input{
		"document": planned["document"], "mode": planned["mode"], "template": planned["template"],
		"acknowledge": take.Acknowledge,
	}
	// A refused plan has no hash: the apply plans it again and answers its refusals.
	if hash, ok := planned["plan_hash"].(string); ok {
		in["plan_hash"] = hash
	}
	if !hub.IsZero() {
		in["hub_id"] = hub.String()
	}
	_, err = d.apply(ctx, actor, in)
	return err
}

// forHubs answers whether the newest version of the template has a hub part.
func (s StarterTemplates) forHubs(ctx context.Context, actor appshared.ActorContext, key string) (bool, error) {
	var found *domain.Template
	err := s.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		templates, err := s.Desk.Store.ListTemplates(ctx)
		if err != nil {
			return err
		}
		presets, err := s.Desk.presetTemplates(ctx, actor)
		if err != nil {
			return err
		}
		all := append(presets, templates...)
		for i := range all {
			if all[i].Key == key && (found == nil || all[i].Version > found.Version) {
				found = &all[i]
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if found == nil {
		return false, domain.ErrTemplateUnknown(key)
	}
	return found.ForHubs(), nil
}
