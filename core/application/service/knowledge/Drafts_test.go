// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var carl = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b0c1")

// readersGate is the shared gate with Carl a reader of every hub who writes nowhere.
type readersGate struct{ gate }

func (g readersGate) allows(actor appshared.ActorContext, request access.Request) bool {
	if actor.AccountID == carl && request.Permission != service.PermissionRead {
		return false
	}
	return g.gate.allows(actor, request)
}

func (g readersGate) Authorize(_ context.Context, actor appshared.ActorContext, request access.Request) error {
	if !g.allows(actor, request) {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func (g readersGate) Permits(_ context.Context, actor appshared.ActorContext, request access.Request) (bool, error) {
	return g.allows(actor, request), nil
}

func (g readersGate) Permitted(_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		request.Path = path
		out[i] = g.allows(actor, request)
	}
	return out, nil
}

type draftStore struct{ drafts map[shared.ID]domain.Draft }

func (d *draftStore) Find(_ context.Context, id shared.ID) (domain.Draft, bool, error) {
	draft, ok := d.drafts[id]
	return draft, ok, nil
}
func (d *draftStore) Put(_ context.Context, draft domain.Draft) error {
	d.drafts[draft.PageID] = draft
	return nil
}
func (d *draftStore) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, ok := d.drafts[id]
	delete(d.drafts, id)
	return ok, nil
}
func (d *draftStore) Pending(context.Context, shared.ID) (int, error) {
	n := 0
	for _, draft := range d.drafts {
		if draft.Pending() {
			n++
		}
	}
	return n, nil
}

func draftsFixture() fixture {
	f := newFixture()
	f.space.Authorizer, f.space.Reader = readersGate{}, readersGate{}
	f.space.Drafting = Drafting{
		Drafts:   &draftStore{drafts: map[shared.ID]domain.Draft{}},
		Settings: &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}},
	}
	return f
}

func asReader(account shared.ID) appshared.ActorContext {
	actor := as(account)
	actor.Scopes = []string{pagesRead}
	return actor
}

// UC-DOC-15: in a space with drafts a new page is its writer's until published, a published
// page's changes wait in a draft its readers do not see - nor its live document - until a writer
// publishes or discards them, and the space goes back to the live document once none waits.
func TestASpaceWithDraftsPublishesWhatItsWritersPrepared(t *testing.T) {
	f := draftsFixture()
	ctx := context.Background()
	if _, err := (SetSpaceDrafts{Space: f.space}).Execute(ctx, as(carl), SetSpaceDraftsCommand{HubID: hub, Drafts: true}); err == nil {
		t.Fatal("a reader switched the space")
	}
	view, err := (SetSpaceDrafts{Space: f.space}).Execute(ctx, as(anna), SetSpaceDraftsCommand{HubID: hub, Drafts: true})
	if err != nil || !domain.DraftsOf(view.Resolution) {
		t.Fatalf("view %+v, %v", view, err)
	}

	// Check 1: a new page waits for its writer.
	page := f.create(t, anna, hub, "", "Handbook", "v1")
	if !page.Restricted {
		t.Fatal("a new page of a space with drafts is open to the space")
	}
	if _, err := (ReadPage{Space: f.space}).Execute(ctx, asReader(carl), page.ID); err == nil {
		t.Fatal("a reader read a page never published")
	}
	if page, err = (PublishPage{Space: f.space}).Execute(ctx, as(anna), PublishPageCommand{PageID: page.ID}); err != nil || page.Restricted {
		t.Fatalf("published %+v, %v", page, err)
	}
	if read, err := (ReadPage{Space: f.space}).Execute(ctx, asReader(carl), page.ID); err != nil || read.Page.Body != "v1" {
		t.Fatalf("read %+v, %v", read, err)
	}

	// Check 2: a published page's change waits in its draft, seen by its writers alone.
	if _, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{
		PageID: page.ID, Change: domain.Change{Body: ptr("v2")}, ExpectedVersion: page.Version,
	}); err != nil {
		t.Fatal(err)
	}
	read, err := (ReadPage{Space: f.space}).Execute(ctx, asReader(carl), page.ID)
	if err != nil || read.Page.Body != "v1" || read.Draft != nil || !read.Drafts {
		t.Fatalf("the reader's %+v, %v", read, err)
	}
	read, err = (ReadPage{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil || read.Page.Body != "v1" || read.Draft == nil || read.Draft.Body != "v2" {
		t.Fatalf("the writer's %+v, %v", read, err)
	}
	place, err := (Locator{Space: f.space}).Locate(ctx, asReader(carl), page.ID)
	if err != nil || place.Read != service.PermissionWriteItems {
		t.Fatalf("live document %+v, %v", place, err)
	}

	// Check 4: back to the live document only once no draft waits.
	if _, err := (SetSpaceDrafts{Space: f.space}).Execute(ctx, as(anna), SetSpaceDraftsCommand{HubID: hub, Drafts: false}); code(err) != "pages.drafts_pending" {
		t.Fatalf("got %v", err)
	}

	// Check 3: publishing makes the draft the page and a version of its history; discarding keeps
	// the page as published.
	versions := len(f.store.versions[page.ID])
	published, err := (PublishPage{Space: f.space}).Execute(ctx, as(bert), PublishPageCommand{PageID: page.ID})
	if err != nil || published.Body != "v2" || len(f.store.versions[page.ID]) <= versions {
		t.Fatalf("published %+v (versions %d), %v", published, len(f.store.versions[page.ID]), err)
	}
	if _, err := (PublishPage{Space: f.space}).Execute(ctx, as(bert), PublishPageCommand{PageID: page.ID}); code(err) != "pages.nothing_to_publish" {
		t.Fatalf("got %v", err)
	}
	if _, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{PageID: page.ID, Change: domain.Change{Body: ptr("v3")}}); err != nil {
		t.Fatal(err)
	}
	if err := (DiscardPageDraft{Space: f.space}).Execute(ctx, as(carl), page.ID); err == nil {
		t.Fatal("a reader discarded a draft")
	}
	if err := (DiscardPageDraft{Space: f.space}).Execute(ctx, as(bert), page.ID); err != nil {
		t.Fatal(err)
	}
	if read, _ := (ReadPage{Space: f.space}).Execute(ctx, as(anna), page.ID); read.Page.Body != "v2" || read.Draft != nil {
		t.Fatalf("after discarding %+v", read)
	}

	if _, err := (SetSpaceDrafts{Space: f.space}).Execute(ctx, as(anna), SetSpaceDraftsCommand{HubID: hub, Drafts: false}); err != nil {
		t.Fatal(err)
	}
	written, err := (UpdatePage{Space: f.space}).Execute(ctx, as(bert), UpdatePageCommand{PageID: page.ID, Change: domain.Change{Body: ptr("v4")}})
	if err != nil || written.Body != "v4" {
		t.Fatalf("live again %+v, %v", written, err)
	}
	if place, _ := (Locator{Space: f.space}).Locate(ctx, asReader(carl), page.ID); place.Read != service.PermissionRead {
		t.Fatalf("live document %+v", place)
	}
}
