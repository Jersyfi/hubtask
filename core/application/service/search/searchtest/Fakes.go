// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package searchtest holds the fakes every kind's finder is tested with: a store answering fixed
// candidates, a unit of work that only calls through, and a reader who reads some hubs.
package searchtest

import (
	"context"
	"strconv"
	"strings"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Names answers the same candidates for every kind; Asks, where set, keeps every question; Keys
// answers a ticket key by its word and number, "sa-12".
type Names struct {
	Rows []searchrepo.Row
	Asks *[]searchrepo.Ask
	Keys map[string]searchrepo.Row
}

// EntryByKey answers the row Keys holds for the key, lowered.
func (n Names) EntryByKey(_ context.Context, key searchrepo.Key, hubID shared.ID) ([]searchrepo.Row, error) {
	row, found := n.Keys[strings.ToLower(key.Word)+"-"+strconv.FormatInt(key.Number, 10)]
	if !found || (!hubID.IsZero() && row.HubID != hubID) {
		return nil, nil
	}
	return []searchrepo.Row{row}, nil
}

func (n Names) all(_ context.Context, ask searchrepo.Ask) ([]searchrepo.Row, error) {
	if n.Asks != nil {
		*n.Asks = append(*n.Asks, ask)
	}
	return n.Rows, nil
}

// Whiteboards answers the candidates.
func (n Names) Whiteboards(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Goals answers the candidates.
func (n Names) Goals(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Dashboards answers the candidates.
func (n Names) Dashboards(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Cis answers the candidates.
func (n Names) Cis(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Portfolios answers the candidates.
func (n Names) Portfolios(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Releases answers the candidates.
func (n Names) Releases(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// Sprints answers the candidates.
func (n Names) Sprints(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// People answers the candidates.
func (n Names) People(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// EntryFiles answers the candidates.
func (n Names) EntryFiles(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// EntryComments answers the candidates.
func (n Names) EntryComments(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// PageFiles answers the candidates.
func (n Names) PageFiles(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// PageComments answers the candidates.
func (n Names) PageComments(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// WhiteboardFiles answers the candidates.
func (n Names) WhiteboardFiles(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// WhiteboardComments answers the candidates.
func (n Names) WhiteboardComments(ctx context.Context, a searchrepo.Ask) ([]searchrepo.Row, error) {
	return n.all(ctx, a)
}

// UnitOfWork calls through.
type UnitOfWork struct{}

// Within calls through.
func (UnitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// WithinReadOnly calls through.
func (UnitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// HubReader reads the hubs it names (and the workspace when Workspace is set), and the paths
// that end on an entry it names; it answers whether a person is a member.
type HubReader struct {
	Hubs      map[shared.ID]bool
	Entries   map[shared.ID]bool
	Workspace bool
	Member    bool
	// Asked is every path asked.
	Asked *[][]identity.Scope
}

// Permitted answers per path.
func (r HubReader) Permitted(_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error) {
	if err := actor.RequireScope(request.TokenScope); err != nil {
		return nil, err
	}
	out := make([]bool, len(paths))
	for i, path := range paths {
		if r.Asked != nil {
			*r.Asked = append(*r.Asked, path)
		}
		last := path[len(path)-1]
		switch {
		case len(path) == 1:
			out[i] = r.Workspace
		case len(r.Entries) > 0:
			out[i] = r.Entries[last.ID]
		default:
			out[i] = r.Hubs[last.ID]
		}
	}
	return out, nil
}

// Permits answers whether the reader is a member.
func (r HubReader) Permits(context.Context, appshared.ActorContext, access.Request) (bool, error) {
	return r.Member, nil
}

// The workspace, the reader and the two hubs every finder test uses: the reader reads HubA and
// not HubB.
var (
	Tenant  = shared.ID("0193a0c0-0000-7000-8000-000000000001")
	Account = shared.ID("0193a0c0-0000-7000-8000-000000000002")
	HubA    = shared.ID("0193a0c0-0000-7000-8000-00000000000a")
	HubB    = shared.ID("0193a0c0-0000-7000-8000-00000000000b")
)

// Actor is a person signed in with the scopes.
func Actor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: Tenant, AccountID: Account, Scopes: scopes}
}

// Row is a candidate in a hub.
func Row(id, hub shared.ID, title string) searchrepo.Row {
	return searchrepo.Row{ID: id, HubID: hub, Title: title}
}
