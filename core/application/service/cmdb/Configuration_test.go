// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// configured adds what the shared memory leaves out: the relation types and the review queue.
type configured struct {
	*memory
	types    []domain.RelationType
	resolved []shared.ID
}

func (c *configured) RelationTypes(context.Context) ([]domain.RelationType, error) {
	return c.types, nil
}

func (c *configured) PutRelationType(_ context.Context, t domain.RelationType, _ int) error {
	for i, existing := range c.types {
		if existing.Key == t.Key {
			c.types[i] = t
			return nil
		}
	}
	c.types = append(c.types, t)
	return nil
}

func (c *configured) Holds(_ context.Context, h shared.ID, _ int) ([]domain.Hold, error) {
	var out []domain.Hold
	for _, hold := range c.holds {
		if hold.HubID == h {
			out = append(out, hold)
		}
	}
	return out, nil
}

func (c *configured) FindHold(_ context.Context, id shared.ID) (domain.Hold, error) {
	for _, hold := range c.holds {
		if hold.ID == id {
			return hold, nil
		}
	}
	return domain.Hold{}, shared.ErrNotFound.WithDetail("cmdb.hold_not_found")
}

func (c *configured) ResolveHold(_ context.Context, id shared.ID, _ time.Time, _ shared.ID) error {
	c.resolved = append(c.resolved, id)
	return nil
}

// structureOnly refuses a write of the configuration to everybody but Anna (rule 2).
type structureOnly struct{ gate }

func (s structureOnly) Authorize(ctx context.Context, actor appshared.ActorContext, r access.Request) error {
	if r.TokenScope == configWrite && actor.AccountID != anna {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return s.gate.Authorize(ctx, actor, r)
}

func configFixture() (fixture, *configured) {
	f := newFixture()
	c := &configured{memory: f.m}
	f.inv.Store = c
	f.inv.Authorizer = structureOnly{}
	return f, c
}

// UC-AST-01: a workspace adds a class of its own beneath a shipped one, with typed attributes and
// identifier rules; the key is not taken twice, a stale change is refused, a shipped class is
// listed first, and only whoever shapes the workspace writes.
func TestAWorkspaceDefinesItsOwnClass(t *testing.T) {
	f, c := configFixture()
	define := DefineCiClass{f.inv}.Descriptor()
	printer := usecase.Input{
		"key": "printer", "parent": "hardware", "name": "Printer", "icon": "printer",
		"attributes": items(
			map[string]any{"key": "toner", "kind": "CHOICE", "options": []any{"black", "colour"}},
			map[string]any{"key": "serial", "kind": "TEXT", "need": "REQUIRED", "identifier": true},
		),
		"identifier_rules": []any{[]any{"serial"}},
	}

	if _, err := call(t, define, as(bert), printer); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("bert defined a class: %v", err)
	}
	if len(c.classes) != 0 {
		t.Fatal("a refused definition wrote a class")
	}
	out, err := call(t, define, as(anna), printer)
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	if out["key"] != "printer" || out["system"] == true {
		t.Fatalf("the class came back as %v", out)
	}
	if last := f.inv.Audit.(*trail).entries; len(last) == 0 || last[len(last)-1].Action != ClassDefinedAction {
		t.Fatal("the definition is not on the trail")
	}

	stale := usecase.Input{"expected_version": 9}
	for k, v := range printer {
		stale[k] = v
	}
	if _, err := call(t, define, as(anna), stale); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale change was written: %v", err)
	}
	if _, err := call(t, define, as(anna), usecase.Input{"key": "server", "parent": "computer", "name": "Mine"}); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a shipped class was changed: %v", err)
	}

	listed, err := call(t, ListCiClasses{f.inv}.Descriptor(), as(bert), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	classes := listed["data"].([]usecase.Output)
	if len(classes) < 2 || classes[0]["system"] != true || classes[len(classes)-1]["key"] != "printer" {
		t.Fatalf("the shipped classes come first, the own one after: %d classes", len(classes))
	}
}

// UC-AST-01: a relation type of the workspace's own says which end suffers; its key is unique,
// it is changed only at the version last read, and archiving keeps it listed.
func TestAWorkspaceDefinesItsOwnRelationType(t *testing.T) {
	f, c := configFixture()
	define := DefineCiRelationType{f.inv}.Descriptor()
	feeds := usecase.Input{
		"key": "feeds", "name": "feeds", "reverse_name": "is fed by", "impact": "TARGET",
		"pairs": items(map[string]any{"from": "server", "to": "server"}),
	}

	if _, err := call(t, define, as(bert), feeds); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("bert defined a relation type: %v", err)
	}
	out, err := call(t, define, as(anna), feeds)
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	if out["impact"] != "TARGET" || len(out["pairs"].([]usecase.Output)) != 1 {
		t.Fatalf("the type came back as %v", out)
	}
	if _, err := call(t, define, as(anna), usecase.Input{
		"key": "feeds", "name": "feeds", "reverse_name": "is fed by", "impact": "TARGET", "expected_version": 5,
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale change was written: %v", err)
	}
	if _, err := call(t, define, as(anna), usecase.Input{
		"key": "feeds", "name": "feeds", "reverse_name": "is fed by", "impact": "TARGET", "archived": true,
	}); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	if len(c.types) != 1 || c.types[0].ArchivedAt == nil {
		t.Fatalf("the archived type: %+v", c.types)
	}
	listed, err := call(t, ListCiRelationTypes{f.inv}.Descriptor(), as(bert), usecase.Input{})
	if err != nil || len(listed["data"].([]usecase.Output)) == 0 {
		t.Fatalf("listing: %v %v", listed, err)
	}
}

// UC-AST-02: a source is defined with its rank and what it may write; `manual` is the people's
// and taken for every source; only whoever shapes the workspace writes one.
func TestASourceIsDefinedWithItsRank(t *testing.T) {
	f, c := configFixture()
	define := DefineCiSource{f.inv}.Descriptor()
	before := len(c.sources)
	scanner := usecase.Input{
		"key": "scanner", "kind": "API", "rank": 120, "classes": []any{"computer"}, "may_create": true,
		"mapping": map[string]any{"class": "server", "name_column": "Host", "columns": map[string]any{"Serial": "serial_number"}},
	}

	if _, err := call(t, define, as(bert), scanner); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("bert defined a source: %v", err)
	}
	if _, err := call(t, define, as(anna), usecase.Input{"key": "manual", "kind": "API", "rank": 10}); code(err) != "cmdb.source_key_taken" {
		t.Fatalf("a source took the people's key: %v", err)
	}
	if len(c.sources) != before {
		t.Fatal("a refused source was written")
	}
	out, err := call(t, define, as(anna), scanner)
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	if out["key"] != "scanner" || out["rank"] != 120 {
		t.Fatalf("the source came back as %v", out)
	}
	listed, err := call(t, ListCiSources{f.inv}.Descriptor(), as(bert), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	// The people's own source is listed first, ahead of the workspace's.
	sources := listed["data"].([]usecase.Output)
	if len(sources) != before+2 || sources[0]["key"] != "manual" || sources[len(sources)-1]["key"] != "scanner" {
		t.Fatalf("the sources: %v", sources)
	}
}

// UC-AST-02 checks 2 and 3: what a reconciliation held and the runs it made are read in the hub
// they belong to; a private hub's are not found by whoever cannot reach it, and dismissing a
// hold is on the trail.
func TestTheReviewQueueBelongsToItsHub(t *testing.T) {
	f, c := configFixture()
	run := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d0a1")
	held := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d0a2")
	secret := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d0a3")
	c.runs = []domain.Run{{ID: run, HubID: private, Source: "scan", StartedAt: start}}
	c.holds = []domain.Hold{
		{ID: held, HubID: hub, Run: run, Source: "scan", Reason: "cmdb.no_identifier", CreatedAt: start},
		{ID: secret, HubID: private, Run: run, Source: "scan", Reason: "cmdb.no_identifier", CreatedAt: start},
	}

	if _, err := call(t, ReadCiRun{f.inv}.Descriptor(), as(bert), usecase.Input{"run_id": run.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("bert read a private hub's run: %v", err)
	}
	if out, err := call(t, ReadCiRun{f.inv}.Descriptor(), as(anna), usecase.Input{"run_id": run.String()}); err != nil || out["id"] != run.String() {
		t.Fatalf("anna reads the run: %v %v", out, err)
	}
	if out, err := call(t, ListCiRuns{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": private.String()}); err != nil || len(out["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the hub's runs: %v %v", out, err)
	}

	listed, err := call(t, ListCiHolds{f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil || len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the hub's holds: %v %v", listed, err)
	}
	if _, err := call(t, DismissCiHold{f.inv}.Descriptor(), as(bert), usecase.Input{"hold_id": secret.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("bert dismissed a private hub's hold: %v", err)
	}
	out, err := call(t, DismissCiHold{f.inv}.Descriptor(), as(bert), usecase.Input{"hold_id": held.String()})
	if err != nil || out["dismissed"] != true || len(c.resolved) != 1 || c.resolved[0] != held {
		t.Fatalf("dismissing: %v %v %v", out, err, c.resolved)
	}
	entries := f.inv.Audit.(*trail).entries
	if entries[len(entries)-1].Action != HoldDismissedAction {
		t.Fatalf("the dismissal is not on the trail: %s", entries[len(entries)-1].Action)
	}
	if out, err := call(t, ListCiDuplicates{f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("bert listed a private hub's duplicates: %v %v", out, err)
	}
}

// registered adds the listing of assets and the removal of an alias to the assurance memory.
type registered struct {
	*assured
	removed []shared.ID
}

func (r *registered) Assets(_ context.Context, h, _ shared.ID, _ int) ([]domain.Asset, error) {
	var out []domain.Asset
	for _, a := range r.assets {
		out = append(out, a)
	}
	return out, nil
}

func (r *registered) RemoveAlias(_ context.Context, id shared.ID) error {
	r.removed = append(r.removed, id)
	return nil
}

// UC-AST-13: an alias is the workspace's configuration - whoever shapes the workspace defines and
// removes one, a stale change is refused, and everybody else may read them.
func TestASoftwareAliasIsTheWorkspacesConfiguration(t *testing.T) {
	f, a := assuredFixture()
	r := &registered{assured: a}
	f.inv.Assurance = r
	f.inv.Authorizer = structureOnly{}
	define := DefineSoftwareAlias{Inventory: f.inv}.Descriptor()
	alias := usecase.Input{"match_product": "code", "publisher": "Microsoft", "product": "Visual Studio Code"}

	if _, err := call(t, define, as(bert), alias); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("bert defined an alias: %v", err)
	}
	if _, err := call(t, define, as(anna), alias); err != nil {
		t.Fatalf("defining: %v", err)
	}
	stale := usecase.Input{"expected_version": 4}
	for k, v := range alias {
		stale[k] = v
	}
	if _, err := call(t, define, as(anna), stale); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale alias was written: %v", err)
	}
	listed, err := call(t, ListSoftwareAliases{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{})
	if err != nil || len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the aliases: %v %v", listed, err)
	}

	id := a.aliases[0].ID.String()
	remove := RemoveSoftwareAlias{Inventory: f.inv}.Descriptor()
	if _, err := call(t, remove, as(bert), usecase.Input{"alias_id": id}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("bert removed an alias: %v", err)
	}
	if _, err := call(t, remove, as(anna), usecase.Input{"alias_id": id}); err != nil || len(r.removed) != 1 {
		t.Fatalf("removing: %v %v", err, r.removed)
	}
	entries := f.inv.Audit.(*trail).entries
	if entries[len(entries)-1].Action != AliasRemovedAction {
		t.Fatalf("the removal is not on the trail: %s", entries[len(entries)-1].Action)
	}
}

// UC-AST-12: a hub's assets are listed with their CIs and read one by one; a CI of a hub out of
// reach has no asset to read, and a CI without one says so.
func TestAHubsAssetsAreReadInTheHub(t *testing.T) {
	f, a := assuredFixture()
	f.inv.Assurance = &registered{assured: a}
	if _, err := call(t, RegisterCiAsset{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"ci_id": host.String(), "asset_tag": "INV-0042", "state": "IN_STOCK",
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}

	list := ListCiAssets{Inventory: f.inv}.Descriptor()
	all, err := call(t, list, as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil || len(all["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the hub's assets: %v %v", all, err)
	}
	none, err := call(t, list, as(bert), usecase.Input{"hub_id": hub.String(), "state": "RETIRED"})
	if err != nil || len(none["data"].([]usecase.Output)) != 0 {
		t.Fatalf("a state no asset is in: %v %v", none, err)
	}
	if _, err := call(t, list, as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("bert listed a private hub's assets: %v", err)
	}

	read := ReadCiAsset{Inventory: f.inv}.Descriptor()
	if out, err := call(t, read, as(bert), usecase.Input{"ci_id": host.String()}); err != nil || out["asset_tag"] != "INV-0042" {
		t.Fatalf("reading the asset: %v %v", out, err)
	}
	if _, err := call(t, read, as(bert), usecase.Input{"ci_id": vault.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("bert read a private hub's asset: %v", err)
	}
	if _, err := call(t, read, as(anna), usecase.Input{"ci_id": vm1.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a CI without an asset: %v", err)
	}
}
