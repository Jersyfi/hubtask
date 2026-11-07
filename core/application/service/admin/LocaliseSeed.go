// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The codes the seeded structure is named from (multi-tenancy.md §5). Provisioning renders them in
// the workspace's language, the owner's redemption in the owner's.
const (
	seedHubName        = "seed.hub.name"
	seedCollectionName = "seed.collection.name"
)

var (
	seedBucketNames = []string{"seed.bucket.todo", "seed.bucket.doing", "seed.bucket.done"}
	seedLabelNames  = []string{
		"seed.label.important", "seed.label.waiting", "seed.label.quick_win", "seed.label.idea",
	}
)

// maxLocaleLength is the contract's bound on a BCP 47 tag; a longer one is no language.
const maxLocaleLength = 35

// seedListSize bounds the containers read: the seed is one hub with one collection, and a person
// who has not yet redeemed has created nothing else.
const seedListSize = 50

// LocaliseSeed renames the defaults provisioning seeded for an owner into the language the owner
// redeemed the invitation in (user-experience.md §4). It runs inside the redemption's transaction,
// in the workspace's scope.
//
// Only what is still exactly as seeded moves: created by this owner, at its first version, under
// the name the workspace's language gives it. Anything somebody renamed, and anybody else's
// redemption, finds nothing to do - so a later owner never renames an established workspace.
type LocaliseSeed struct {
	Tenants    adminrepo.Tenants
	Containers workrepo.Containers
	Buckets    workrepo.Buckets
	Labels     workrepo.Labels
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Renderer   i18n.Renderer
	Text       text.Normalizer
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// LocaliseSeed implements identity.SeedLocaliser.
func (l LocaliseSeed) LocaliseSeed(
	ctx context.Context, owner domain.Account, locale string, now time.Time,
) error {
	if locale == "" || len(locale) > maxLocaleLength {
		return nil
	}
	tenant, err := l.Tenants.Find(ctx)
	if err != nil {
		return err
	}
	if locale == tenant.DefaultLocale {
		return nil
	}
	names := seedNames{renderer: l.Renderer, Text: l.Text, from: tenant.DefaultLocale, to: locale}
	actor := event.Actor{Kind: shared.ActorUser, ID: owner.ID}

	hubs, err := l.Containers.List(ctx, workrepo.ContainerQuery{
		Type: work.ContainerHub, Page: workrepo.Page{Size: seedListSize},
	})
	if err != nil {
		return err
	}
	for _, hub := range hubs.Containers {
		if hub.CreatedBy != owner.ID {
			continue
		}
		if err := l.renameContainer(ctx, hub, names, seedHubName, actor, now); err != nil {
			return err
		}
		collections, err := l.Containers.List(ctx, workrepo.ContainerQuery{
			ParentID: hub.ID, Type: work.ContainerCollection, Page: workrepo.Page{Size: seedListSize},
		})
		if err != nil {
			return err
		}
		for _, collection := range collections.Containers {
			if collection.CreatedBy != owner.ID {
				continue
			}
			if err := l.renameContainer(ctx, collection, names, seedCollectionName, actor, now); err != nil {
				return err
			}
			if err := l.renameBuckets(ctx, collection.ID, names, actor, now); err != nil {
				return err
			}
			if err := l.renameLabels(ctx, collection.ID, names, actor, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l LocaliseSeed) renameContainer(
	ctx context.Context, container work.Container, names seedNames, code string,
	actor event.Actor, now time.Time,
) error {
	wanted, ok := names.untouched(container.Version, container.Name, code)
	if !ok {
		return nil
	}
	after, changes, err := container.Renamed(work.ContainerAttributes{Name: &wanted}, l.Text, now)
	if err != nil || len(changes) == 0 {
		return err
	}
	if err := l.Containers.SetAttributes(ctx, after, container.Version); err != nil {
		return err
	}
	after.Version = container.Version + 1
	announcement, err := event.NewContainerRenamed(l.IDs.NewID(), after, changes, actor, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := l.Events.Append(ctx, announcement); err != nil {
		return err
	}
	// The container itself is the visibility filter, as for every rename (offline-sync.md §3.1).
	return l.record(ctx, after.TenantID, "container", after.ID, after.ID, actor.ID, changes)
}

func (l LocaliseSeed) renameBuckets(
	ctx context.Context, collectionID shared.ID, names seedNames, actor event.Actor, now time.Time,
) error {
	buckets, err := l.Buckets.List(ctx, collectionID)
	if err != nil {
		return err
	}
	for _, bucket := range buckets {
		for _, code := range seedBucketNames {
			wanted, ok := names.untouched(bucket.Version, bucket.Name, code)
			if !ok {
				continue
			}
			after, changes, err := bucket.Updated(work.BucketAttributes{Name: &wanted}, l.Text)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				break
			}
			if err := l.Buckets.SetAttributes(ctx, after, bucket.Version); err != nil {
				return err
			}
			after.Version = bucket.Version + 1
			announcement, err := event.NewBucketUpdated(l.IDs.NewID(), after, changes, actor, now, event.Cause{})
			if err != nil {
				return err
			}
			if err := l.Events.Append(ctx, announcement); err != nil {
				return err
			}
			if err := l.record(ctx, after.TenantID, "bucket", after.ID, collectionID, actor.ID, changes); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (l LocaliseSeed) renameLabels(
	ctx context.Context, collectionID shared.ID, names seedNames, actor event.Actor, now time.Time,
) error {
	labels, err := l.Labels.List(ctx, collectionID)
	if err != nil {
		return err
	}
	for _, label := range labels {
		for _, code := range seedLabelNames {
			wanted, ok := names.untouched(label.Version, label.Name, code)
			if !ok {
				continue
			}
			after, changes, err := label.Updated(work.LabelAttributes{Name: &wanted}, l.Text)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				break
			}
			if err := l.Labels.SetAttributes(ctx, after, label.Version); err != nil {
				return err
			}
			after.Version = label.Version + 1
			announcement, err := event.NewLabelUpdated(l.IDs.NewID(), after, changes, actor, now, event.Cause{})
			if err != nil {
				return err
			}
			if err := l.Events.Append(ctx, announcement); err != nil {
				return err
			}
			if err := l.record(ctx, after.TenantID, "label", after.ID, collectionID, actor.ID, changes); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// record writes one change entry per field that moved, each with its own HLC, as every update
// does (offline-sync.md §4.2).
func (l LocaliseSeed) record(
	ctx context.Context, tenantID shared.ID, entity string, entityID, visibility, actorID shared.ID,
	changes []work.FieldChange,
) error {
	for _, change := range changes {
		if err := l.Changes.Record(ctx, changelog.Change{
			TenantID: tenantID, Entity: entity, EntityID: entityID, Op: changelog.Upsert,
			ContainerID: visibility, ActorID: actorID, HLC: l.HLC.Next(),
			Field: change.Field, Payload: map[string]any{change.Field: change.To},
		}); err != nil {
			return err
		}
	}
	return nil
}

// seedNames tells a seeded name nobody touched, and gives it in the owner's language.
type seedNames struct {
	renderer i18n.Renderer
	Text     text.Normalizer
	from, to string
}

func (n seedNames) untouched(version int, name, code string) (string, bool) {
	if version != 1 || name != n.render(n.from, code) {
		return "", false
	}
	wanted := n.render(n.to, code)
	return wanted, wanted != name
}

func (n seedNames) render(locale, code string) string {
	rendered := n.renderer.Render(locale, code, nil)
	if n.Text != nil {
		return n.Text.NFC(rendered)
	}
	return rendered
}
