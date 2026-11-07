// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"fmt"
	"slices"

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
	_ work.HubTemplates     = StarterTemplates{}
	_ work.StarterSamples   = StarterTemplates{}
)

// AddStarterSamples makes a starter collection's sample entries as AddSampleEntry makes a
// template's - the entry created as any is, then marked under a key of the hub - so the hub's
// *Remove sample entries* takes them too. The key names the collection's place in the set and the
// title's, which a set made once never repeats. Called inside the starter's transaction, which
// authorised the actor; creating each entry asks what creating any entry asks.
func (s StarterTemplates) AddStarterSamples(ctx context.Context, actor appshared.ActorContext, hub, collection shared.ID, index int, titles []string) error {
	for i, title := range titles {
		in := usecase.Input{
			"hub_id": hub.String(), "collection_id": collection.String(), "title": title,
			"key": fmt.Sprintf("starter_%d_%d", index+1, i+1),
		}
		if _, err := s.Desk.addSample(ctx, actor, hub, in); err != nil {
			return err
		}
	}
	return nil
}

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

// TakeWorkspacePartOnce applies the template's workspace part for a hub made from it, where the
// workspace has not taken the template: one it took stays at the version it took, since making a
// hub is no upgrade. A template without a hub part is refused, as its hub's plan refuses it.
func (s StarterTemplates) TakeWorkspacePartOnce(ctx context.Context, actor appshared.ActorContext, take work.StarterTemplate) error {
	found, err := s.newest(ctx, actor, take.Key)
	if err != nil {
		return err
	}
	if !found.ForHubs() {
		return refused("config.template_not_for_hubs", "/template", map[string]string{"template": take.Key})
	}
	if len(found.Document) == 0 {
		return nil
	}
	var taken bool
	err = s.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		uses, err := s.Desk.Store.TemplateUses(ctx, "")
		taken = slices.ContainsFunc(uses, func(u domain.TemplateUse) bool { return u.TemplateKey == take.Key })
		return err
	})
	if err != nil || taken {
		return err
	}
	return s.take(ctx, actor, take, "")
}

// forHubs answers whether the newest version of the template has a hub part.
func (s StarterTemplates) forHubs(ctx context.Context, actor appshared.ActorContext, key string) (bool, error) {
	found, err := s.newest(ctx, actor, key)
	if err != nil {
		return false, err
	}
	return found.ForHubs(), nil
}

// newest is the newest version of the template, Hubtask's presets included.
func (s StarterTemplates) newest(ctx context.Context, actor appshared.ActorContext, key string) (*domain.Template, error) {
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
		return nil, err
	}
	if found == nil {
		return nil, domain.ErrTemplateUnknown(key)
	}
	return found, nil
}
