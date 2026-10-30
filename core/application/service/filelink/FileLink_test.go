// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filelink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filelink"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e001")
	anna       = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e002")
	bert       = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e003")
	hub        = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e004")
	entry      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e006")
	collection = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e008")
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

// gate lets Anna write; Bert only reads.
type gate struct{}

func (gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if actor.AccountID == bert && r.Permission != service.PermissionRead {
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	return nil
}

type entries struct{}

func (entries) Entries(_ context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range ids {
		if id == entry {
			out = append(out, workrepo.EntrySummary{ID: id, HubID: hub, CollectionID: collection})
		}
	}
	return out, nil
}

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, ch changelog.Change) error {
	c.recorded = append(c.recorded, ch)
	return nil
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

type memory struct {
	repository.Store
	links []domain.Link
}

func (m *memory) List(_ context.Context, item shared.ID) ([]domain.Link, error) {
	var out []domain.Link
	for _, l := range m.links {
		if l.ItemID == item {
			out = append(out, l)
		}
	}
	return out, nil
}
func (m *memory) Count(ctx context.Context, item shared.ID) (int, error) {
	l, err := m.List(ctx, item)
	return len(l), err
}
func (m *memory) Find(_ context.Context, id shared.ID) (domain.Link, error) {
	for _, l := range m.links {
		if l.ID == id {
			return l, nil
		}
	}
	return domain.Link{}, shared.ErrNotFound.WithDetail("filelink.not_found")
}
func (m *memory) Add(_ context.Context, link domain.Link) (bool, error) {
	for _, l := range m.links {
		if l.ItemID == link.ItemID && l.Address == link.Address {
			return false, nil
		}
	}
	m.links = append(m.links, link)
	return true, nil
}
func (m *memory) Remove(_ context.Context, id shared.ID) error {
	for i, l := range m.links {
		if l.ID == id {
			m.links = append(m.links[:i], m.links[i+1:]...)
			return nil
		}
	}
	return shared.ErrNotFound.WithDetail("filelink.not_found")
}

func newFiles() (Files, *memory, *changes, *trail) {
	m, c, a := &memory{}, &changes{}, &trail{}
	return Files{
		Store: m, Entries: entries{}, Authorizer: gate{}, Audit: a, UnitOfWork: passthrough{},
		Clock: clockAt{start}, IDs: &counter{}, Changes: c, HLC: hlc{},
	}, m, c, a
}

func as(who shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: who}
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

// A writer links a Nextcloud file on an entry, its store told by its address; a reader sees it, a
// reader may not link one, and the same address is named once (UC-INT-14 checks 1, 2).
func TestAWriterLinksAFileAndAReaderSeesIt(t *testing.T) {
	f, _, ch, tr := newFiles()
	ctx := context.Background()
	link := LinkEntryFile{f}.Descriptor().Handler
	in := usecase.Input{"item_id": entry.String(), "address": "https://cloud.example.org/s/aBcD1234", "name": "Login flow"}
	if _, err := link.Invoke(ctx, as(bert), in); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a reader linked a file: %v", err)
	}
	out, err := link.Invoke(ctx, as(anna), in)
	if err != nil {
		t.Fatal(err)
	}
	if out["provider"] != "NEXTCLOUD" || out["name"] != "Login flow" || out["collection_id"] != collection.String() {
		t.Errorf("linked %v", out)
	}
	if _, err := link.Invoke(ctx, as(anna), in); detail(err) != "filelink.already_linked" {
		t.Errorf("the same address twice: %v", err)
	}
	list, err := ListEntryFileLinks{f}.Descriptor().Handler.Invoke(ctx, as(bert), usecase.Input{"item_id": entry.String()})
	if err != nil {
		t.Fatal(err)
	}
	if rows := list["data"].([]usecase.Output); len(rows) != 1 || rows[0]["address"] != "https://cloud.example.org/s/aBcD1234" {
		t.Errorf("listed %v", list)
	}
	if len(ch.recorded) != 1 || ch.recorded[0].Entity != Entity || ch.recorded[0].ContainerID != collection {
		t.Errorf("change log %+v", ch.recorded)
	}
	for _, e := range tr.entries {
		if strings.Contains(fmt.Sprint(e.Changes), "Login") || strings.Contains(fmt.Sprint(e.Changes), "cloud.example.org") {
			t.Errorf("the trail holds the file's name or address: %v", e.Changes)
		}
	}
}

// An entry holds at most fifty links, and a link taken off is gone from the entry and from the
// device's copy (UC-INT-14 checks 3, 4).
func TestALinkIsTakenOffAndAnEntryHoldsAtMostFifty(t *testing.T) {
	f, m, ch, _ := newFiles()
	ctx := context.Background()
	link := LinkEntryFile{f}.Descriptor().Handler
	var first string
	for i := 0; i < domain.MaxLinksPerEntry; i++ {
		out, err := link.Invoke(ctx, as(anna), usecase.Input{"item_id": entry.String(),
			"address": fmt.Sprintf("https://contoso.sharepoint.com/sites/web/Doc%d.docx", i)})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = out["id"].(string)
		}
	}
	if _, err := link.Invoke(ctx, as(anna), usecase.Input{"item_id": entry.String(),
		"address": "https://contoso.sharepoint.com/sites/web/One-too-many.docx"}); detail(err) != "filelink.too_many" {
		t.Errorf("the fifty-first link: %v", err)
	}
	unlink := UnlinkEntryFile{f}.Descriptor().Handler
	if _, err := unlink.Invoke(ctx, as(bert), usecase.Input{"link_id": first}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader took a link off: %v", err)
	}
	if _, err := unlink.Invoke(ctx, as(anna), usecase.Input{"link_id": first}); err != nil {
		t.Fatal(err)
	}
	if len(m.links) != domain.MaxLinksPerEntry-1 {
		t.Errorf("%d links left", len(m.links))
	}
	if last := ch.recorded[len(ch.recorded)-1]; last.Op != changelog.Delete || last.EntityID.String() != first {
		t.Errorf("the device was not told: %+v", last)
	}
	if _, err := unlink.Invoke(ctx, as(anna), usecase.Input{"link_id": first}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("taken off twice: %v", err)
	}
}
