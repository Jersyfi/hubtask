// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// StructureResolver answers the structure in force for a container: its own layer, its hub's, the
// workspace's, held inside the installation's bound (domain-model.md §2). Every reader and writer of
// a structure resolves through it, so a value read on one door is the value written on another.
type StructureResolver struct {
	Containers repository.Containers
	Structures repository.Structures
	Profiles   metarepo.CapabilityProfiles
	// Presets reads the saved presets a layer can be copied from. A build without it offers
	// Hubtask's presets alone.
	Presets repository.Presets
}

// Wired reports whether the resolver has what it needs. A handler built without one - in a test
// of something else - answers containers without their structure rather than failing.
func (r StructureResolver) Wired() bool {
	return r.Containers != nil && r.Structures != nil && r.Profiles != nil
}

// layerReading is one transaction's view of the layers: the bound, the workspace and each hub read once,
// however many containers are resolved from them.
type layerReading struct {
	resolver  StructureResolver
	hold      repository.LayerHold
	bound     *domain.StructureBound
	workspace *domain.WorkspaceStructure
	version   int
	hubs      map[shared.ID]domain.Container
}

// within starts a reading inside the caller's transaction. A shared or exclusive hold takes the
// workspace's row and every hub's row as it reads them, so a writer of those layers and this one
// take turns (domain-model.md §2).
func (r StructureResolver) within(hold repository.LayerHold) *layerReading {
	return &layerReading{resolver: r, hold: hold, hubs: map[shared.ID]domain.Container{}}
}

func (s *layerReading) Bound(ctx context.Context) (domain.StructureBound, error) {
	if s.bound == nil {
		bound, err := structureBound(ctx, s.resolver.Profiles)
		if err != nil {
			return domain.StructureBound{}, err
		}
		s.bound = &bound
	}
	return *s.bound, nil
}

// Workspace answers the workspace's layer and the tenant row's version.
func (s *layerReading) Workspace(ctx context.Context) (domain.WorkspaceStructure, int, error) {
	if s.workspace == nil {
		workspace, version, err := s.resolver.Structures.Workspace(ctx, s.hold)
		if err != nil {
			return domain.WorkspaceStructure{}, 0, err
		}
		s.workspace, s.version = &workspace, version
	}
	return *s.workspace, s.version, nil
}

func (s *layerReading) hub(ctx context.Context, id shared.ID) (domain.Container, error) {
	if hub, read := s.hubs[id]; read {
		return hub, nil
	}
	if s.hold != repository.LayerRead {
		if err := s.resolver.Structures.HoldContainer(ctx, id); err != nil {
			return domain.Container{}, err
		}
	}
	hub, err := s.resolver.Containers.Find(ctx, id)
	if err != nil {
		return domain.Container{}, err
	}
	s.hubs[id] = hub
	return hub, nil
}

// layersAbove are the layers between the workspace and the container itself: a collection's hub.
func (s *layerReading) layersAbove(ctx context.Context, container domain.Container) ([]domain.LayerAt, error) {
	if container.Type != domain.ContainerCollection || container.ParentID.IsZero() {
		return nil, nil
	}
	hub, err := s.hub(ctx, container.ParentID)
	if err != nil {
		return nil, err
	}
	return []domain.LayerAt{{Source: identity.SourceHub, Layer: hub.Structure}}, nil
}

// ownSource is the source a container's own layer answers as.
func ownSource(container domain.Container) identity.PolicySource {
	if container.Type == domain.ContainerCollection {
		return identity.SourceCollection
	}
	return identity.SourceHub
}

// Of resolves the structure in force for the container.
func (s *layerReading) Of(ctx context.Context, container domain.Container) (domain.EffectiveStructure, error) {
	return s.with(ctx, container, container.Structure)
}

// Above resolves what the container would have without a layer of its own: what a lock is judged
// against and what a reset falls back to.
func (s *layerReading) Above(ctx context.Context, container domain.Container) (domain.EffectiveStructure, error) {
	return s.with(ctx, container, domain.StructureLayer{})
}

func (s *layerReading) with(
	ctx context.Context, container domain.Container, own domain.StructureLayer,
) (domain.EffectiveStructure, error) {
	bound, err := s.Bound(ctx)
	if err != nil {
		return domain.EffectiveStructure{}, err
	}
	workspace, _, err := s.Workspace(ctx)
	if err != nil {
		return domain.EffectiveStructure{}, err
	}
	layers, err := s.layersAbove(ctx, container)
	if err != nil {
		return domain.EffectiveStructure{}, err
	}
	layers = append(layers, domain.LayerAt{Source: ownSource(container), Layer: own})
	return domain.ResolveStructure(bound, workspace, layers...), nil
}

// Resolve resolves a page of containers in one read-only transaction of its own. Called after the
// permission question was answered, so it resolves only what the caller may see.
func (r StructureResolver) Resolve(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork, containers []domain.Container,
) (map[shared.ID]domain.EffectiveStructure, error) {
	resolved := make(map[shared.ID]domain.EffectiveStructure, len(containers))
	if !r.Wired() || len(containers) == 0 {
		return resolved, nil
	}
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		reading := r.within(repository.LayerRead)
		for _, container := range containers {
			effective, err := reading.Of(ctx, container)
			if err != nil {
				return err
			}
			resolved[container.ID] = effective
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// withStructure adds the structure in force to a container's output, where it was resolved.
func withStructure(out map[string]any, structures map[shared.ID]domain.EffectiveStructure, id shared.ID) map[string]any {
	if effective, resolved := structures[id]; resolved {
		out["structure"] = effectiveStructureOutput(effective)
	}
	return out
}

// WorkspaceStructureAnswer answers the workspace's own structure for `GET /tenant`, in its own
// read-only transaction: what every hub and collection inherits, the locks, and the default preset.
type WorkspaceStructureAnswer struct {
	Resolver   StructureResolver
	UnitOfWork persistence.UnitOfWork
}

// WorkspaceStructure is the WorkspaceStructure schema.
func (a WorkspaceStructureAnswer) WorkspaceStructure(
	ctx context.Context, actor appshared.ActorContext,
) (map[string]any, error) {
	var out map[string]any
	err := a.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		reading := a.Resolver.within(repository.LayerRead)
		bound, err := reading.Bound(ctx)
		if err != nil {
			return err
		}
		workspace, _, err := reading.Workspace(ctx)
		if err != nil {
			return err
		}
		out = workspaceStructureOutput(domain.ResolveStructure(bound, workspace), workspace)
		return nil
	})
	return out, err
}
