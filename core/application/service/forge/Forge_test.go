// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package forge

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/forge"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f001")
	anna       = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f002")
	bert       = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f003")
	hub        = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f004")
	otherHub   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f005")
	entry      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f006")
	elsewhere  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f007")
	collection = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000f008")
	start      = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clockAt struct{ now time.Time }

func (c clockAt) Now() time.Time { return c.now }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", c.n))
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, e audit.Entry) error {
	t.entries = append(t.entries, e)
	return nil
}

// gate lets Anna do everything; Bert only reads.
type gate struct{}

func (gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if actor.AccountID == bert && r.Permission != service.PermissionRead {
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	return nil
}

type hubs struct{ workrepo.Containers }

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, Type: work.ContainerHub}, nil
}

type keys map[string]shared.ID

func (k keys) ItemByKey(_ context.Context, key mail.Key) (shared.ID, error) {
	if id, ok := k[key.String()]; ok {
		return id, nil
	}
	return "", shared.ErrNotFound
}

type entries struct{}

func (entries) Entries(_ context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range ids {
		switch id {
		case entry:
			out = append(out, workrepo.EntrySummary{ID: id, HubID: hub, CollectionID: collection})
		case elsewhere:
			out = append(out, workrepo.EntrySummary{ID: id, HubID: otherHub, CollectionID: collection})
		}
	}
	return out, nil
}

type secrets struct{}

func (secrets) Derive(t, h shared.ID, salt string) string {
	return "s/" + t.String() + "/" + h.String() + "/" + salt
}

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, ch changelog.Change) error {
	c.recorded = append(c.recorded, ch)
	return nil
}

// draws answers other bytes at every draw, as the machine's entropy does.
type draws struct{ n byte }

func (d *draws) Bytes(n int) ([]byte, error) {
	d.n++
	return clock.FixedEntropy{Seed: d.n}.Bytes(n)
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

type memory struct {
	repository.Store
	hooks map[shared.ID]domain.Hook
	links map[shared.ID]domain.Link
	seen  map[string]bool
}

func (m *memory) ListHooks(_ context.Context, h shared.ID) ([]domain.Hook, error) {
	var out []domain.Hook
	for _, x := range m.hooks {
		if x.HubID == h {
			out = append(out, x)
		}
	}
	return out, nil
}
func (m *memory) CountHooks(context.Context, shared.ID) (int, error) { return len(m.hooks), nil }
func (m *memory) FindHook(_ context.Context, id shared.ID) (domain.Hook, error) {
	h, ok := m.hooks[id]
	if !ok {
		return h, shared.ErrNotFound.WithDetail("forge.hook_not_found")
	}
	return h, nil
}
func (m *memory) PutHook(_ context.Context, h domain.Hook) error { m.hooks[h.ID] = h; return nil }
func (m *memory) SeeDelivery(_ context.Context, h shared.ID, d []byte, _, _ time.Time) (bool, error) {
	k := h.String() + string(d)
	if m.seen[k] {
		return false, nil
	}
	m.seen[k] = true
	return true, nil
}
func (m *memory) FindLink(_ context.Context, item, hook shared.ID, kind domain.LinkKind, repo, ref string) (domain.Link, error) {
	for _, l := range m.links {
		if l.ItemID == item && l.HookID == hook && l.Kind == kind && l.Repository == repo && l.Ref == ref {
			return l, nil
		}
	}
	return domain.Link{}, shared.ErrNotFound
}
func (m *memory) PutLink(_ context.Context, l domain.Link) error { m.links[l.ID] = l; return nil }
func (m *memory) ListLinks(_ context.Context, item shared.ID, _ int) ([]domain.Link, error) {
	var out []domain.Link
	for _, l := range m.links {
		if l.ItemID == item {
			out = append(out, l)
		}
	}
	return out, nil
}
func newForge() (Forge, *memory, *changes) {
	m := &memory{hooks: map[shared.ID]domain.Hook{}, links: map[shared.ID]domain.Link{}, seen: map[string]bool{}}
	c := &changes{}
	return Forge{
		Store: m, Containers: hubs{}, Keys: keys{"WEB-42": entry, "OPS-7": elsewhere}, Entries: entries{},
		Secrets: secrets{}, Authorizer: gate{}, Audit: &trail{}, UnitOfWork: passthrough{},
		Clock: clockAt{start}, IDs: &counter{}, Entropy: &draws{}, Changes: c, HLC: hlc{},
	}, m, c
}

func as(who shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: who}
}

// parseAll is a parser that answers what the test hands it, whatever the format.
func parseAll(sigs ...domain.Signal) Parser {
	return func(domain.Kind, string, []byte) ([]domain.Signal, error) { return sigs, nil }
}

func create(t *testing.T, f Forge, kind domain.Kind) (shared.ID, string) {
	t.Helper()
	out, err := CreateCodeHook{f}.Descriptor().Handler.Invoke(context.Background(), as(anna),
		usecase.Input{"hub_id": hub.String(), "kind": string(kind), "name": "web"})
	if err != nil {
		t.Fatal(err)
	}
	return shared.ID(out["id"].(string)), out["secret"].(string)
}

// UC-INT-13 check 1: a hook is added under STRUCTURE, its secret answered once and never read back.
func TestAHookIsAddedByAStructureHolderAndReadWithoutItsSecret(t *testing.T) {
	f, _, _ := newForge()
	if _, err := (CreateCodeHook{f}).Descriptor().Handler.Invoke(context.Background(), as(bert),
		usecase.Input{"hub_id": hub.String(), "kind": "GITHUB", "name": "web"}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a reader added a hook: %v", err)
	}
	id, secret := create(t, f, domain.GitHub)
	if secret == "" {
		t.Fatal("no secret answered")
	}
	out, err := ListCodeHooks{f}.Descriptor().Handler.Invoke(context.Background(), as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil {
		t.Fatal(err)
	}
	rows := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["id"] != id.String() {
		t.Fatalf("hooks = %v", rows)
	}
	if _, has := rows[0]["secret"]; has {
		t.Fatal("a read answered the secret")
	}
	rotated, err := RotateCodeHookSecret{f}.Descriptor().Handler.Invoke(context.Background(), as(anna), usecase.Input{"hook_id": id.String()})
	if err != nil || rotated["secret"] == secret {
		t.Fatalf("rotation = %v, %v", rotated["secret"], err)
	}
}

// UC-INT-13 checks 2 and 4: a signed delivery links the hub's entry its key names, an entry of
// another hub is not linked, a wrong signature and a repeated delivery are one 404.
func TestADeliveryLinksTheHubsEntryByItsKey(t *testing.T) {
	f, m, ch := newForge()
	id, secret := create(t, f, domain.GitHub)
	at := start.Add(-time.Minute)
	parse := parseAll(
		domain.Signal{Kind: domain.PullRequest, Repository: "team/web", Ref: "7", Title: "Login", State: domain.Open,
			URL: "https://forge.example/team/web/pull/7", At: &at, Texts: []string{"Login", "feature/WEB-42-login"}},
		domain.Signal{Kind: domain.Commit, Repository: "team/web", Ref: "abc", Title: "OPS-7 and WEB-404", At: &at,
			Texts: []string{"OPS-7 and WEB-404"}},
	)
	body := []byte(`{"any":"thing"}`)
	if _, err := f.Deliver(context.Background(), Delivery{TenantID: tenant, HookID: id, Signature: Sign("wrong", body), Body: body}, parse); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a wrong signature was taken: %v", err)
	}
	d := Delivery{TenantID: tenant, HookID: id, Signature: Sign(secret, body), DeliveryID: "d-1", Body: body}
	r, err := f.Deliver(context.Background(), d, parse)
	if err != nil {
		t.Fatal(err)
	}
	if r.Received != 2 || r.Linked != 1 || r.Unknown != 2 {
		t.Fatalf("receipt = %+v", r)
	}
	if len(m.links) != 1 {
		t.Fatalf("links = %+v", m.links)
	}
	if len(ch.recorded) != 1 || ch.recorded[0].Entity != LinkEntity || ch.recorded[0].ContainerID != collection {
		t.Fatalf("change log = %+v", ch.recorded)
	}
	if _, err := f.Deliver(context.Background(), d, parse); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a repeated delivery was taken: %v", err)
	}
	out, err := ListEntryCodeLinks{f}.Descriptor().Handler.Invoke(context.Background(), as(bert), usecase.Input{"item_id": entry.String()})
	if err != nil {
		t.Fatal(err)
	}
	rows := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["state"] != "OPEN" || rows[0]["kind"] != "PULL_REQUEST" {
		t.Fatalf("an entry's links = %v", rows)
	}
}

// UC-INT-13 check 4: GitLab proves the secret by its token; a hook switched off takes nothing.
func TestAGitLabTokenIsTheSecretAndAHookSwitchedOffTakesNothing(t *testing.T) {
	f, _, _ := newForge()
	id, secret := create(t, f, domain.GitLab)
	parse := parseAll(domain.Signal{Kind: domain.Branch, Repository: "team/web", Ref: "WEB-42-x", Texts: []string{"WEB-42-x"}})
	if _, err := f.Deliver(context.Background(), Delivery{TenantID: tenant, HookID: id, Token: "nope", Body: []byte("{}")}, parse); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a wrong token was taken: %v", err)
	}
	if r, err := f.Deliver(context.Background(), Delivery{TenantID: tenant, HookID: id, Token: secret, DeliveryID: "1", Body: []byte("{}")}, parse); err != nil || r.Linked != 1 {
		t.Fatalf("the token was refused: %+v, %v", r, err)
	}
	if _, err := (UpdateCodeHook{f}).Descriptor().Handler.Invoke(context.Background(), as(anna),
		usecase.Input{"hook_id": id.String(), "enabled": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Deliver(context.Background(), Delivery{TenantID: tenant, HookID: id, Token: secret, DeliveryID: "2", Body: []byte("{}")}, parse); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a hook switched off took a delivery: %v", err)
	}
}
