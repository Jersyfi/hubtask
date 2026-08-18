// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	GetContainerName   = "GetContainer"
	ListContainersName = "ListContainers"
	containersRead     = "containers:read"

	// ContainerReadAction is the audit code of an attempted read. Declared even though an ordinary
	// read writes no entry: a *refused* read does, and it is recorded against the action that was
	// refused rather than against a generic "denied" (audit.md §4, access.Request.Action).
	ContainerReadAction audit.Action = "container.read"
)

// Reader is the slice of the authorisation service the read side needs beyond Authorizer: the same
// question asked of a whole page at once.
//
// Two interfaces rather than one with two methods, because the two reads need different halves - a
// single Get asks about one path, and only the unanchored list needs the plural form. A use case
// declaring a dependency it does not use is a use case whose test has to satisfy it anyway.
type Reader interface {
	Permitted(
		ctx context.Context, actor appshared.ActorContext, request access.Request,
		paths [][]identity.Scope,
	) ([]bool, error)
}

// GetContainer reads one hub or collection.
//
// Read-only throughout, which is not a detail: the transaction may be served by a read replica
// (multi-tenancy.md §7), and a read that opened a write transaction would pin every list in the
// product to the primary.
type GetContainer struct {
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// GetContainerQuery is the input, typed.
type GetContainerQuery struct {
	ContainerID shared.ID
}

// Execute returns the container.
func (h GetContainer) Execute(
	ctx context.Context, actor appshared.ActorContext, query GetContainerQuery,
) (domain.Container, error) {
	// The row is read before the permission question can be asked, because the answer depends on it:
	// a membership held at the hub applies downwards, so the path has to be known first
	// (domain-model.md §3.2). Nothing is disclosed by the order - the tenant boundary is row level
	// security, which has already applied, and what is read here is returned only if the check below
	// passes.
	var container domain.Container
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Containers.Find(ctx, query.ContainerID)
		if err != nil {
			return err
		}
		container = found
		return nil
	})
	if err != nil {
		return domain.Container{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     ContainerReadAction,
		TokenScope: containersRead,
		TargetType: containerTarget,
		TargetID:   container.ID,
	}); err != nil {
		return domain.Container{}, err
	}
	return container, nil
}

// Descriptor is the catalogue entry.
func (h GetContainer) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetContainerName,
		Summary: "Reads one hub or collection by its identifier. An archived or trashed container " +
			"is returned as it is, with the timestamp saying so, rather than reported as missing.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID, Required: true,
				Description: "The hub or collection to read.",
			},
		},
		// Required is false and the action is still named: an ordinary read is not an auditable
		// event (audit.md §4 lists none), and a refused one is - recorded by the authorisation
		// service against this action.
		Audit: usecase.AuditDeclaration{
			Action: ContainerReadAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetContainer) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	container, err := h.Execute(ctx, actor, GetContainerQuery{ContainerID: containerID})
	if err != nil {
		return nil, err
	}
	return containerOutput(container), nil
}

// ListContainers reads one level of the container tree.
type ListContainers struct {
	Containers repository.Containers
	Authorizer Authorizer
	Reader     Reader
	UnitOfWork persistence.UnitOfWork
}

// ListContainersQuery is the input, typed.
type ListContainersQuery struct {
	// ParentID is the hub whose collections are wanted. Empty means the hubs, which is the one level
	// anchored to nothing.
	ParentID shared.ID
	// Type narrows the level; empty means both kinds.
	Type            domain.ContainerType
	IncludeArchived bool
	Cursor          string
	Size            int
}

// Execute returns one page of the level.
//
// The permission check has two shapes, because the two levels are different questions.
//
// A level under a named hub is a question about that hub: the client named it, so a refusal is a
// refusal, and one check covers every row in it - a membership at the hub applies to all its
// collections. Answering an empty page instead would tell a client that has no access the same thing
// as one whose hub is empty.
//
// The hub level is anchored to nothing, so there is no single path to check. Checking the tenant scope
// would refuse everybody whose membership sits on a hub rather than on the workspace, which is most of
// what hub-scoped memberships are for. So the page is read and then narrowed to the hubs the actor may
// see: "what can I see" is answered with what they can see, not with a 403.
func (h ListContainers) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListContainersQuery,
) (repository.ContainerPage, error) {
	if !query.ParentID.IsZero() {
		if err := h.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionRead,
			Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(query.ParentID)},
			Action:     ContainerReadAction,
			TokenScope: containersRead,
			TargetType: containerTarget,
			TargetID:   query.ParentID,
		}); err != nil {
			return repository.ContainerPage{}, err
		}
	}

	var page repository.ContainerPage
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Containers.List(ctx, repository.ContainerQuery{
			ParentID:        query.ParentID,
			Type:            query.Type,
			IncludeArchived: query.IncludeArchived,
			Page:            repository.Page{Cursor: query.Cursor, Size: PageSize(query.Size)},
		})
		return err
	})
	if err != nil {
		return repository.ContainerPage{}, err
	}

	if query.ParentID.IsZero() {
		if page.Containers, err = h.visible(ctx, actor, page.Containers); err != nil {
			return repository.ContainerPage{}, err
		}
	}
	return page, nil
}

// visible drops the containers the actor may not read.
//
// The page's own cursor is left alone deliberately. It is a boundary in the *scanned* set, not in the
// filtered one, which is what keeps the walk correct: a client sees shorter pages, and pages on until
// has_more is false. Narrowing the cursor to the last visible row would skip everything between it and
// the last row actually read.
func (h ListContainers) visible(
	ctx context.Context, actor appshared.ActorContext, containers []domain.Container,
) ([]domain.Container, error) {
	if len(containers) == 0 {
		return containers, nil
	}

	paths := make([][]identity.Scope, 0, len(containers))
	for _, container := range containers {
		paths = append(paths, containerPath(container))
	}

	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ContainerReadAction,
		TokenScope: containersRead,
		TargetType: containerTarget,
	}, paths)
	if err != nil {
		return nil, err
	}

	kept := make([]domain.Container, 0, len(containers))
	for i, container := range containers {
		if i < len(allowed) && allowed[i] {
			kept = append(kept, container)
		}
	}
	return kept, nil
}

// Descriptor is the catalogue entry.
func (h ListContainers) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListContainersName,
		Summary: "Lists one level of the workspace tree in its manual order: the hubs when no " +
			"parent is given, that hub's collections when one is. Paged with an opaque cursor; " +
			"a page carries the cursor for the next one until there are none left.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "parent_id", Kind: usecase.KindID,
				Description: "The hub whose collections are wanted. Omitted for the hubs themselves.",
			},
			{
				Name: "type", Kind: usecase.KindString,
				Enum: []string{string(domain.ContainerHub), string(domain.ContainerCollection)},
				Description: "Narrows the level to one kind. A collection always has a parent and a " +
					"hub never does, so the impossible combinations return an empty page.",
			},
			{
				Name: "include_archived", Kind: usecase.KindBool,
				Description: "Keeps archived containers in the page. Trashed ones are never in it.",
			},
			{
				Name: "cursor", Kind: usecase.KindString,
				Description: "The next_cursor of the previous page. Opaque: it is produced by this " +
					"server and is not to be constructed or parsed.",
			},
			{
				Name: "size", Kind: usecase.KindInt,
				Description: "Rows per page, 1 to 200. Defaults to 50.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ContainerReadAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListContainers) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	parentID, err := in.ID("parent_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, ListContainersQuery{
		ParentID:        parentID,
		Type:            domain.ContainerType(in.String("type")),
		IncludeArchived: in.Bool("include_archived"),
		Cursor:          in.String("cursor"),
		Size:            in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	data := make([]usecase.Output, 0, len(page.Containers))
	for _, container := range page.Containers {
		data = append(data, containerOutput(container))
	}
	return pageOutput(data, page.Info), nil
}

// containerPath is the authorisation path to a container, running from the tenant downwards: the hub
// it sits in, then the container itself. A membership at any of them counts, which is what "the
// effective permission is the highest role along the path" means (domain-model.md §3.2).
func containerPath(container domain.Container) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !container.ParentID.IsZero() {
		path = append(path, identity.HubScope(container.ParentID))
	}
	if container.Type == domain.ContainerHub {
		return append(path, identity.HubScope(container.ID))
	}
	return append(path, identity.CollectionScope(container.ID))
}
