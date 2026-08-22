// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package meta answers what this installation is and what it can do.
package meta

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
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
	// Limits are the numbers a client has to respect to avoid being refused.
	Limits map[string]int64
	// Features says which optional parts of the installation are configured.
	Features map[string]bool
}

// GetCapabilities reads the manifest.
//
// The item types come from the database, never from a constant here: a tenant may narrow a
// profile, and a copy in code would answer the default while the database answered the override.
type GetCapabilities struct {
	Profiles   repository.CapabilityProfiles
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

	var profiles []work.CapabilityProfile
	err := g.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		profiles, err = g.Profiles.List(ctx)
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
