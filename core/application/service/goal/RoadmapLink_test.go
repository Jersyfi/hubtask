// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"bytes"
	"context"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// roadmapLinks keeps the links by the token's whole text, as the adapter keeps them by its hash.
type roadmapLinks struct {
	byPortfolio map[shared.ID]domain.RoadmapLink
	token       map[shared.ID]string
}

func (l *roadmapLinks) Put(_ context.Context, link domain.RoadmapLink, token integration.RoadmapLinkToken) error {
	l.byPortfolio[link.PortfolioID], l.token[link.PortfolioID] = link, token.Secret()
	return nil
}
func (l *roadmapLinks) Find(_ context.Context, id shared.ID) (domain.RoadmapLink, bool, error) {
	link, ok := l.byPortfolio[id]
	return link, ok, nil
}
func (l *roadmapLinks) FindByToken(_ context.Context, token integration.RoadmapLinkToken) (domain.RoadmapLink, error) {
	for id, held := range l.token {
		if held == token.Secret() {
			return l.byPortfolio[id], nil
		}
	}
	return domain.RoadmapLink{}, shared.ErrNotFound
}
func (l *roadmapLinks) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, ok := l.byPortfolio[id]
	delete(l.byPortfolio, id)
	delete(l.token, id)
	return ok, nil
}

type switches struct {
	plan   setting.Layer
	stored []settingrepo.Stored
}

func (s *switches) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance}, s.plan, nil
}
func (s *switches) At(context.Context, []settingrepo.Place, string) ([]settingrepo.Stored, error) {
	return s.stored, nil
}
func (s *switches) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key setting.Key, entry setting.Entry, _ time.Time) error {
	s.stored = []settingrepo.Stored{{Place: place, Key: key, Entry: entry}}
	return nil
}
func (s *switches) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) {
	return false, nil
}

type makers struct {
	identityrepo.Accounts
	status map[shared.ID]identity.AccountStatus
}

func (m makers) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	status, ok := m.status[id]
	if !ok {
		return identity.Account{}, shared.ErrNotFound
	}
	return identity.Account{ID: id, TenantID: tenantID, Kind: identity.AccountUser, Status: status}, nil
}

type entropy struct{ fill byte }

func (e *entropy) Bytes(n int) ([]byte, error) {
	e.fill++
	return bytes.Repeat([]byte{e.fill}, n), nil
}

type goalsOn struct{ off bool }

func (g *goalsOn) On(context.Context, appshared.ActorContext, shared.ID) (module.Set, error) {
	if g.off {
		return module.Set{}, nil
	}
	return module.Set{module.Goals}, nil
}

// UC-GOL-03 check 15 (the owner's fourth answers, item 9): a portfolio's roadmap gets a public link
// only where the workspace allows it, by who manages the workspace's members; whoever holds the
// link reads the roadmap without an account, as its maker reads it now, and a new link, the
// switch off, the module off, the maker gone or the link taken away all answer the same not found.
func TestARoadmapsPublicLinkOpensItReadOnlyWhileTheWorkspaceAllowsIt(t *testing.T) {
	s, _, g, _ := newSteward(t)
	s.Schedules = plans{}
	store, held := &switches{}, &roadmapLinks{byPortfolio: map[shared.ID]domain.RoadmapLink{}, token: map[shared.ID]string{}}
	people := makers{status: map[shared.ID]identity.AccountStatus{ana: identity.AccountActive, ben: identity.AccountActive}}
	modules := &goalsOn{}
	trail := &sink{}
	s.Audit = trail
	s.Public = RoadmapLinking{Links: held, Settings: store, Entropy: &entropy{}, Accounts: people, Modules: modules}

	created := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{
		"name": "Programme", "items": []any{
			map[string]any{"container_id": listA.String()}, map[string]any{"container_id": listS.String()},
		},
	})
	id := created.String("id")
	in := usecase.Input{"portfolio_id": id}

	if _, err := call(t, CreatePortfolioPublicLink{s}.Descriptor(), ana, in); code(err) != "portfolios.public_links_off" {
		t.Fatalf("a link while the workspace allows none: %v", err)
	}
	if _, err := call(t, SetPublicRoadmapsPolicy{s}.Descriptor(), ben, usecase.Input{"allowed": true}); err == nil {
		t.Fatal("somebody who does not manage the members switched public roadmaps on")
	}
	agent := as(ana)
	agent.Kind = appshared.ActorAIAgent
	if _, err := (SetPublicRoadmapsPolicy{s}).Execute(context.Background(), agent, true); code(err) != "portfolios.agent_decides_policy_not" {
		t.Fatalf("an agent decided the policy: %v", err)
	}
	if out := ok(t, SetPublicRoadmapsPolicy{s}.Descriptor(), ana, usecase.Input{"allowed": true}); out["allowed"] != true {
		t.Fatalf("the policy after switching on: %v", out)
	}
	if _, err := call(t, CreatePortfolioPublicLink{s}.Descriptor(), ben, in); err == nil {
		t.Fatal("Ben, who does not manage the members, made a link")
	}
	if len(g.refused) == 0 {
		t.Fatal("the refusal was not the authoriser's")
	}
	minted := ok(t, CreatePortfolioPublicLink{s}.Descriptor(), ana, in)
	token, err := integration.ParseRoadmapLinkToken(minted.String("token"))
	if err != nil {
		t.Fatalf("the minted token does not read back: %v", err)
	}
	if state := ok(t, ReadPortfolioPublicLink{s}.Descriptor(), ana, in); state["link"] == nil {
		t.Fatalf("the link is not answered: %v", state)
	} else if _, leaked := state["link"].(usecase.Output)["token"]; leaked {
		t.Fatal("reading the link answered its token")
	}
	var created2 bool
	for _, e := range trail.entries {
		created2 = created2 || e.Action == PortfolioPublicLinkCreatedAction
	}
	if !created2 {
		t.Fatal("making the link wrote no audit entry")
	}

	read := ReadPublicRoadmap{s}
	road, err := read.Execute(context.Background(), token, "")
	if err != nil {
		t.Fatalf("the link does not open: %v", err)
	}
	if lanes := road["lanes"].([]usecase.Output); len(lanes) != 2 || road["name"] != "Programme" {
		t.Fatalf("the roadmap the link shows: %v", road)
	}

	gone := func(why string) {
		t.Helper()
		if _, err := read.Execute(context.Background(), token, ""); code(err) != "portfolios.link_not_found" {
			t.Fatalf("%s: the link still answers (%v)", why, err)
		}
	}
	// Read as the maker: Ben's link shows only what Ben reads.
	held.byPortfolio[shared.ID(id)] = domain.RoadmapLink{TenantID: tenantID, PortfolioID: shared.ID(id), CreatedBy: ben, CreatedAt: noon}
	if road, err := read.Execute(context.Background(), token, ""); err != nil || len(road["lanes"].([]usecase.Output)) != 1 {
		t.Fatalf("a link made by Ben shows %v (%v)", road, err)
	}
	people.status[ben] = identity.AccountDisabled
	gone("its maker is disabled")
	people.status[ben] = identity.AccountActive
	held.byPortfolio[shared.ID(id)] = domain.RoadmapLink{TenantID: tenantID, PortfolioID: shared.ID(id), CreatedBy: ana, CreatedAt: noon}

	modules.off = true
	gone("the goals module is off")
	modules.off = false

	ok(t, SetPublicRoadmapsPolicy{s}.Descriptor(), ana, usecase.Input{"allowed": false})
	gone("public roadmaps are switched off")
	ok(t, SetPublicRoadmapsPolicy{s}.Descriptor(), ana, usecase.Input{"allowed": true})

	other, err := integration.NewRoadmapLinkToken(shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000ff"), bytes.Repeat([]byte{9}, integration.RoadmapLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read.Execute(context.Background(), other, ""); code(err) != "portfolios.link_not_found" {
		t.Fatalf("an unknown token: %v", err)
	}

	ok(t, CreatePortfolioPublicLink{s}.Descriptor(), ana, in)
	gone("a new link replaced it")

	if _, err := call(t, DeletePortfolioPublicLink{s}.Descriptor(), ana, in); err != nil {
		t.Fatalf("taking the link away: %v", err)
	}
	if _, err := call(t, DeletePortfolioPublicLink{s}.Descriptor(), ana, in); code(err) != "portfolios.link_not_found" {
		t.Fatalf("taking a missing link away: %v", err)
	}
	if d := (DeletePortfolioPublicLink{s}).Descriptor(); !d.Destructive || d.Audit.Action != PortfolioPublicLinkDeletedAction || !d.Audit.Required {
		t.Fatalf("the removal is not audited as destructive: %+v", d.Audit)
	}
}

// A plan that offers no public roadmaps locks the switch off: the workspace cannot turn it on.
func TestAPlanLocksPublicRoadmapsOff(t *testing.T) {
	s, _, _, _ := newSteward(t)
	locked := setting.Layer{Level: setting.LevelPlan, Entries: map[setting.Key]setting.Entry{
		domain.PublicRoadmapsKey: {Value: false, Hold: setting.HoldLock},
	}}
	s.Public = RoadmapLinking{Settings: &switches{plan: locked}}
	if _, err := call(t, SetPublicRoadmapsPolicy{s}.Descriptor(), ana, usecase.Input{"allowed": true}); err == nil {
		t.Fatalf("a locked switch was set: %v", err)
	}
}
