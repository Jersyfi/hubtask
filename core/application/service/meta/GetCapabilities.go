// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package meta answers what this installation is and what it can do.
package meta

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// APIVersion is the one major path of the contract (api-guidelines.md §8).
const APIVersion = "v1"

// Capabilities is the self-description clients configure themselves from instead of hard-coding
// values (api-guidelines.md §1).
//
// What is deliberately absent: view layouts, automation triggers and event types. They are
// declared in the schema and will be answered by the tasks that build them. An empty list would
// read as "this installation has none", which is a different statement from "this part of the
// contract is not implemented yet" - and the first of those is a lie a client would act on.
type Capabilities struct {
	ProductVersion string
	APIVersion     string
	TenancyMode    string
	ItemTypes      []work.CapabilityProfile
	// QueryFields is what POST /items:query accepts: which fields may be filtered, ordered and
	// grouped by, and with which operators (B-12, api-guidelines.md §3).
	//
	// The catalogue itself rather than a copy of it, for the reason the item types come from the
	// database: a manifest that listed a field the grammar refuses would send a client to build a
	// filter editor for a query that cannot run.
	QueryFields []view.Field
	// TextLanguages are the languages this installation can index the text of, as BCP 47 tags
	// (C-08, ADR-0034).
	//
	// Read from the database for the reason the item types are: it is the installation's answer
	// rather than the product's, because which text search configurations exist is what its
	// PostgreSQL was built with. A client's language picker is then this list - and an entry
	// written in a language that is not in it is stored and searched word by word rather than
	// refused, which is why this is a manifest entry and not a validation rule.
	TextLanguages []string
	// Roles is the role matrix as this installation enforces it (domain-model.md §3.2).
	//
	// Read from the matrix rather than restated here, for the reason the item types come from the
	// database: a second copy answers what the first one used to say. It matters most for the two
	// cells no permission name carries - a contributor writes only what is assigned to them, a
	// guest comments on what it may not change - because a client that does not know them draws
	// buttons the server refuses (C-04).
	Roles []RoleDescription
	// Limits are the numbers a client has to respect to avoid being refused.
	Limits map[string]int64
	// Features says which optional parts of the installation are configured.
	Features map[string]bool
}

// RoleDescription is one row of that matrix: the columns the role carries unqualified, and how far
// it reaches into a single entry.
type RoleDescription struct {
	Role        identity.Role
	Permissions []service.Permission
	// ItemAccess is answered for every kind of access, including the ones that are AccessNone: an
	// absent key would leave a client guessing, and guessing wrong in the permissive direction is
	// the mistake this whole endpoint exists to prevent.
	ItemAccess map[service.ItemAction]service.ItemAccess
}

// roleMatrix reads the matrix out of the domain service, one row per defined role.
func roleMatrix() []RoleDescription {
	roles := identity.Roles()
	described := make([]RoleDescription, 0, len(roles))

	for _, role := range roles {
		reach := make(map[service.ItemAction]service.ItemAccess, len(service.ItemActions()))
		for _, action := range service.ItemActions() {
			reach[action] = service.ItemAccessOf(role, action)
		}
		described = append(described, RoleDescription{
			Role:        role,
			Permissions: service.PermissionsOf(role),
			ItemAccess:  reach,
		})
	}
	return described
}

// GetCapabilities reads the manifest.
//
// The item types come from the database, never from a constant here: a tenant may narrow a
// profile, and a copy in code would answer the default while the database answered the override.
type GetCapabilities struct {
	Profiles   repository.CapabilityProfiles
	Languages  repository.TextLanguages
	UnitOfWork persistence.UnitOfWork
	Config     env.Config
}

// Execute answers for the actor, who may be anonymous.
//
// An anonymous caller reads the installation scope: the system-defined profiles, which belong to
// no tenant. That is not a hole in the boundary but the strictest position inside it - with no
// tenant context set, every policy comparing against one is false, so no tenant's rows are
// reachable at all (multi-tenancy.md §2.1). It is what lets a client configure itself before it
// has signed in, which is what the endpoint is for (api-guidelines.md §1).
func (g GetCapabilities) Execute(ctx context.Context, actor appshared.ActorContext) (Capabilities, error) {
	scope := persistence.InstallationScope()
	if actor.IsAuthenticated() {
		scope = actor.PersistenceScope()
	}

	var (
		profiles  []work.CapabilityProfile
		languages []string
	)
	err := g.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		if profiles, err = g.Profiles.List(ctx); err != nil {
			return err
		}
		// In the same transaction as the profiles, because it is the same question asked of the
		// same installation - and because a second one would be a second round trip for a manifest
		// a client reads before it has signed in.
		languages, err = g.Languages.List(ctx)
		return err
	})
	if err != nil {
		return Capabilities{}, err
	}

	return Capabilities{
		ProductVersion: g.Config.Version,
		APIVersion:     APIVersion,
		TenancyMode:    string(g.Config.Tenancy),
		ItemTypes:      profiles,
		QueryFields:    view.Fields(),
		TextLanguages:  languages,
		Roles:          roleMatrix(),
		Limits: map[string]int64{
			"max_body_bytes":            g.Config.Request.MaxBodyBytes,
			"max_upload_bytes":          g.Config.Request.MaxUploadBytes,
			"rate_limit_per_minute":     int64(g.Config.RateLimit.TokenPerMinute),
			"anonymous_rate_per_minute": int64(g.Config.RateLimit.AnonymousPerMinute),
		},
		Features: map[string]bool{
			// What is configured, not what is implemented. A client uses this to decide whether
			// to offer an action at all - offering "send by email" on an installation with no
			// SMTP server is a dead end the manifest can prevent.
			"mail":    g.Config.Mail.Host != "",
			"storage": g.Config.Storage.Kind != "",
			"tracing": g.Config.Tracing.Enabled,
			// Whether this installation serves the web interface at "/" (ADR-0028). A client
			// discovers it here rather than by asking for "/" and reading the answer, which is
			// the same reason every other optional part of the installation is in this map.
			"web_ui": g.Config.UI.Enabled,
		},
	}, nil
}
