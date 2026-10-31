// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b001")
	anna    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b002")
	bert    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b003")
	hub     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b004")
	private = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b005")
	start   = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type movingClock struct{ at time.Time }

func (c *movingClock) Now() time.Time { return c.at }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", c.n))
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, entry audit.Entry) error {
	t.entries = append(t.entries, entry)
	return nil
}

// gate lets everybody do anything at the shared hub and nobody but Anna anything at the private one.
type gate struct{}

func (gate) allows(actor appshared.ActorContext, request access.Request) bool {
	for _, scope := range request.Path {
		if scope.ID == private && actor.AccountID != anna {
			return false
		}
	}
	return true
}

func (g gate) Authorize(_ context.Context, actor appshared.ActorContext, request access.Request) error {
	if !g.allows(actor, request) {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func (g gate) Permits(_ context.Context, actor appshared.ActorContext, request access.Request) (bool, error) {
	return g.allows(actor, request), nil
}

func (g gate) Permitted(_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		request.Path = path
		out[i] = g.allows(actor, request)
	}
	return out, nil
}

type hubs struct{ workrepo.Containers }

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id != hub && id != private {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return work.Container{ID: id, Type: work.ContainerHub}, nil
}

func (hubs) List(context.Context, workrepo.ContainerQuery) (workrepo.ContainerPage, error) {
	return workrepo.ContainerPage{Containers: []work.Container{{ID: hub}, {ID: private}}}, nil
}

// store is the page store in memory.
type store struct {
	pages    map[shared.ID]domain.Page
	versions map[shared.ID][]domain.Version
	comments map[shared.ID]domain.Comment
	// restrictedTo, shares and personal are what Access_test.go's methods keep.
	restrictedTo map[shared.ID][]shared.ID
	shares       map[shared.ID][]domain.Share
	personal     map[shared.ID]shared.ID
	// media is what AttachMedia keeps, by page.
	media map[shared.ID][]shared.ID
	// labels is what SetLabels keeps, by page.
	labels map[shared.ID][]string
}

func newStore() *store {
	return &store{
		pages: map[shared.ID]domain.Page{}, versions: map[shared.ID][]domain.Version{}, comments: map[shared.ID]domain.Comment{},
		restrictedTo: map[shared.ID][]shared.ID{}, shares: map[shared.ID][]domain.Share{}, personal: map[shared.ID]shared.ID{},
		labels: map[shared.ID][]string{},
	}
}

func (s *store) Insert(_ context.Context, page domain.Page) error {
	s.pages[page.ID] = page
	return nil
}
func (s *store) Find(_ context.Context, id shared.ID) (domain.Page, error) {
	page, ok := s.pages[id]
	if !ok {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return page, nil
}
func (s *store) FindForUpdate(ctx context.Context, id shared.ID) (domain.Page, error) {
	return s.Find(ctx, id)
}
func (s *store) Update(_ context.Context, page domain.Page, expected int) error {
	if s.pages[page.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	page.Version = expected + 1
	s.pages[page.ID] = page
	return nil
}
func (s *store) List(_ context.Context, hubID shared.ID, archived, templates bool, _ int) ([]domain.Page, error) {
	var out []domain.Page
	for _, page := range s.pages {
		if page.HubID == hubID && !page.IsTrashed() && (archived || !page.IsArchived()) && (!templates || page.Template) {
			out = append(out, page)
		}
	}
	slices.SortFunc(out, func(a, b domain.Page) int { return strings.Compare(a.OrderKey, b.OrderKey) })
	return out, nil
}
func (s *store) Trashed(context.Context, shared.ID, int) ([]domain.Page, error) { return nil, nil }
func (s *store) Count(context.Context, shared.ID) (int, error)                  { return len(s.pages), nil }
func (s *store) Ancestors(_ context.Context, id shared.ID) ([]repository.Ancestor, error) {
	var out []repository.Ancestor
	for parent := s.pages[id].ParentID; !parent.IsZero(); parent = s.pages[parent].ParentID {
		out = append(out, repository.Ancestor{ID: parent, Title: s.pages[parent].Title})
	}
	return out, nil
}
func (s *store) Subtree(_ context.Context, id shared.ID, _ bool, _ int) ([]domain.Page, error) {
	out := []domain.Page{s.pages[id]}
	for _, page := range s.pages {
		if page.ParentID == id {
			out = append(out, page)
		}
	}
	return out, nil
}
func (s *store) LastOrderKey(_ context.Context, hubID, parentID shared.ID) (string, error) {
	last := ""
	for _, page := range s.pages {
		if page.HubID == hubID && page.ParentID == parentID && page.OrderKey > last {
			last = page.OrderKey
		}
	}
	return last, nil
}
func (s *store) Trash(_ context.Context, id, _ shared.ID, at time.Time) (int, error) {
	count := 0
	for key, page := range s.pages {
		if key == id || page.ParentID == id {
			page.DeletedAt = &at
			s.pages[key] = page
			count++
		}
	}
	return count, nil
}
func (s *store) Restore(_ context.Context, id, _ shared.ID, _, _ time.Time) (int, error) {
	page := s.pages[id]
	page.DeletedAt = nil
	s.pages[id] = page
	return 1, nil
}
func (s *store) Search(_ context.Context, q repository.PageQuery) ([]repository.Hit, error) {
	var out []repository.Hit
	for _, page := range s.pages {
		if slices.Contains(q.HubIDs, page.HubID) && strings.Contains(page.Body, q.Text) &&
			(q.Label == "" || slices.Contains(s.labels[page.ID], q.Label)) &&
			(q.AuthorID.IsZero() || page.CreatedBy == q.AuthorID) &&
			(q.UpdatedFrom == nil || !page.UpdatedAt.Before(*q.UpdatedFrom)) &&
			(q.UpdatedTo == nil || page.UpdatedAt.Before(*q.UpdatedTo)) {
			out = append(out, repository.Hit{ID: page.ID, HubID: page.HubID, Title: page.Title})
		}
	}
	return out, nil
}
func (s *store) Labels(_ context.Context, ids []shared.ID) (map[shared.ID][]string, error) {
	out := map[shared.ID][]string{}
	for _, id := range ids {
		if labels := s.labels[id]; len(labels) > 0 {
			out[id] = slices.Clone(labels)
		}
	}
	return out, nil
}
func (s *store) SetLabels(_ context.Context, id shared.ID, labels []string) error {
	s.labels[id] = slices.Clone(labels)
	return nil
}
func (s *store) SpaceLabels(_ context.Context, hubID shared.ID, _ int) ([]repository.PageLabel, error) {
	var out []repository.PageLabel
	for id, labels := range s.labels {
		if page := s.pages[id]; page.HubID == hubID && !page.IsTrashed() && !page.IsArchived() {
			for _, label := range labels {
				out = append(out, repository.PageLabel{PageID: id, Label: label})
			}
		}
	}
	slices.SortFunc(out, func(a, b repository.PageLabel) int { return strings.Compare(string(a.PageID), string(b.PageID)) })
	return out, nil
}
func (s *store) Summaries(context.Context, []shared.ID) ([]repository.Summary, error) {
	return nil, nil
}
func (s *store) Backlinks(context.Context, string, shared.ID, int) ([]repository.Backlink, error) {
	return nil, nil
}
func (s *store) LastVersion(_ context.Context, id shared.ID) (domain.Version, bool, error) {
	versions := s.versions[id]
	if len(versions) == 0 {
		return domain.Version{}, false, nil
	}
	return versions[len(versions)-1], true, nil
}
func (s *store) InsertVersion(_ context.Context, v domain.Version) error {
	s.versions[v.PageID] = append(s.versions[v.PageID], v)
	return nil
}
func (s *store) FoldVersion(_ context.Context, v domain.Version) error {
	versions := s.versions[v.PageID]
	versions[len(versions)-1] = v
	return nil
}
func (s *store) NameVersion(_ context.Context, id shared.ID, seq int, name string) error {
	s.versions[id][seq-1].Name = name
	return nil
}
func (s *store) Versions(context.Context, shared.ID, int, int) ([]repository.VersionRow, error) {
	return nil, nil
}
func (s *store) Version(_ context.Context, id shared.ID, seq int) (domain.Version, error) {
	if seq < 1 || seq > len(s.versions[id]) {
		return domain.Version{}, shared.ErrNotFound.WithDetail("pages.version_not_found")
	}
	return s.versions[id][seq-1], nil
}
func (s *store) InsertComment(_ context.Context, c domain.Comment) error {
	s.comments[c.ID] = c
	return nil
}
func (s *store) FindComment(_ context.Context, id shared.ID) (domain.Comment, error) {
	c, ok := s.comments[id]
	if !ok {
		return domain.Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
	}
	return c, nil
}
func (s *store) Comments(context.Context, shared.ID, int) ([]domain.Comment, error) { return nil, nil }
func (s *store) UpdateComment(_ context.Context, c domain.Comment, _ int) error {
	s.comments[c.ID] = c
	return nil
}

type fixture struct {
	space Space
	store *store
	clock *movingClock
	trail *trail
}

func newFixture() fixture {
	f := fixture{store: newStore(), clock: &movingClock{at: start}, trail: &trail{}}
	f.space = Space{
		Pages: f.store, Containers: hubs{}, Authorizer: gate{}, Reader: gate{}, Audit: f.trail,
		UnitOfWork: passthrough{}, Clock: f.clock, IDs: &counter{},
	}
	return f
}

func as(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenant, AccountID: account, Kind: appshared.ActorUser, Scopes: []string{pagesRead, pagesWrite}}
}

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func (f fixture) create(t *testing.T, account, hubID, parent shared.ID, title, body string) domain.Page {
	t.Helper()
	page, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(account), CreatePageCommand{
		HubID: hubID, ParentID: parent, Title: title, Body: &body,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	return page
}

// UC-DOC-01: a page written in a hub is found again by its words, by whoever may read the hub, and
// not by anybody else; a page's words reach the trail only as a sensitive change.
func TestAPageWrittenInAHubIsFoundAgain(t *testing.T) {
	f := newFixture()
	page := f.create(t, anna, hub, "", "Handbook", "Our travel budget")
	hidden := f.create(t, anna, private, "", "Notes", "my own budget")
	hits, err := (SearchPages{Space: f.space}).Execute(context.Background(), as(bert), SearchPagesQuery{Text: "budget"})
	if err != nil || len(hits) != 1 || hits[0].ID != page.ID {
		t.Fatalf("Bert found %+v, %v; want only the shared page", hits, err)
	}
	if _, err := (ReadPage{Space: f.space}).Execute(context.Background(), as(bert), hidden.ID); err == nil {
		t.Fatal("Bert read a page of Anna's private hub")
	}
	own, _ := (SearchPages{Space: f.space}).Execute(context.Background(), as(anna), SearchPagesQuery{Text: "budget"})
	if len(own) != 2 {
		t.Fatalf("Anna found %d pages; want both", len(own))
	}
	for _, entry := range f.trail.entries {
		if body, ok := entry.Changes[domain.FieldBody].(map[string]any); ok {
			if _, open := body["to"]; open {
				t.Fatal("a page's body reached the trail as open text")
			}
		}
	}
}

// A page sits under another; moving one under itself or its own child is refused.
func TestAPageCannotMoveUnderItself(t *testing.T) {
	f := newFixture()
	root := f.create(t, anna, hub, "", "Handbook", "")
	child := f.create(t, anna, hub, root.ID, "Onboarding", "")
	read, err := (ReadPage{Space: f.space}).Execute(context.Background(), as(bert), child.ID)
	if err != nil || len(read.Ancestors) != 1 || read.Ancestors[0]["id"] != root.ID.String() || !read.Writable {
		t.Fatalf("read %+v, %v", read, err)
	}
	_, err = (MovePage{Space: f.space}).Execute(context.Background(), as(anna), MovePageCommand{PageID: root.ID, ParentID: child.ID})
	if code(err) != "pages.parent_cycle" {
		t.Fatalf("got %v; want pages.parent_cycle", err)
	}
	second := f.create(t, anna, hub, "", "Policies", "")
	moved, err := (MovePage{Space: f.space}).Execute(context.Background(), as(anna), MovePageCommand{PageID: second.ID, ParentID: root.ID, AfterID: child.ID})
	if err != nil || moved.ParentID != root.ID || moved.OrderKey <= child.OrderKey {
		t.Fatalf("moved %+v, %v", moved, err)
	}
}

// UC-DOC-06: one person's writes within a session are one version; another person's write and a
// restore are versions of their own, and restoring brings the old words back.
func TestAPagesHistoryKeepsOneVersionPerSession(t *testing.T) {
	f := newFixture()
	page := f.create(t, anna, hub, "", "Plan", "one")
	write := func(account shared.ID, body string) {
		t.Helper()
		f.clock.at = f.clock.at.Add(time.Minute)
		if _, err := (UpdatePage{Space: f.space}).Execute(context.Background(), as(account), UpdatePageCommand{
			PageID: page.ID, Change: domain.Change{Body: &body},
		}); err != nil {
			t.Fatal(err)
		}
	}
	write(anna, "two")
	write(anna, "three")
	write(bert, "four")
	versions := f.store.versions[page.ID]
	if len(versions) != 2 || versions[0].Body != "three" || versions[1].Body != "four" {
		t.Fatalf("versions %+v", versions)
	}
	restored, err := (RestorePageVersion{Space: f.space}).Execute(context.Background(), as(anna), page.ID, 1, 0)
	if err != nil || restored.Body != "three" {
		t.Fatalf("restored %+v, %v", restored, err)
	}
	versions = f.store.versions[page.ID]
	if len(versions) != 3 || versions[2].Restored != 1 {
		t.Fatalf("the restore is not a version of its own: %+v", versions)
	}
}

// UC-DOC-05: a page started from a template carries the template's body; a page that is not a
// template is refused as one.
func TestAPageStartsFromATemplate(t *testing.T) {
	f := newFixture()
	template, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(anna), CreatePageCommand{
		HubID: hub, Title: "Meeting notes", Template: true, Body: ptr("## Agenda\n\n## Decisions\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(bert), CreatePageCommand{HubID: hub, Title: "Weekly", TemplateID: template.ID})
	if err != nil || page.Body != template.Body || page.Template {
		t.Fatalf("page %+v, %v", page, err)
	}
	plain := f.create(t, anna, hub, "", "Plain", "x")
	if _, err := (CreatePage{Space: f.space}).Execute(context.Background(), as(bert), CreatePageCommand{HubID: hub, Title: "y", TemplateID: plain.ID}); code(err) != "pages.template_not_template" {
		t.Fatalf("got %v", err)
	}
}

// A comment's words are its author's to change; anyone who may write the page resolves a thread.
func TestACommentIsItsAuthorsAndItsThreadEverybodys(t *testing.T) {
	f := newFixture()
	page := f.create(t, anna, hub, "", "Plan", "the budget")
	comment, err := (AddPageComment{Space: f.space}).Execute(context.Background(), as(anna), AddPageCommentCommand{
		PageID: page.ID, Body: "Why so high?", Anchor: "block-1", Quote: "the budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (ChangePageComment{Space: f.space}).Execute(context.Background(), as(bert), ChangePageCommentCommand{CommentID: comment.ID, Body: ptr("x")}); code(err) != "pages.comment_not_own" {
		t.Fatalf("got %v; want pages.comment_not_own", err)
	}
	resolved, err := (ChangePageComment{Space: f.space}).Execute(context.Background(), as(bert), ChangePageCommentCommand{CommentID: comment.ID, Resolved: ptrBool(true)})
	if err != nil || resolved.ResolvedBy != bert {
		t.Fatalf("resolved %+v, %v", resolved, err)
	}
}

// The live document of a page is judged at its hub, as the page.
func TestAPagesLiveDocumentSitsAtItsHub(t *testing.T) {
	f := newFixture()
	page := f.create(t, anna, private, "", "Notes", "")
	place, err := (Locator{Space: f.space}).Locate(context.Background(), as(anna), page.ID)
	if err != nil || len(place.Path) != 2 || place.Path[1].ID != private || place.Write != service.PermissionWriteItems {
		t.Fatalf("place %+v, %v", place, err)
	}
}

func ptr(s string) *string { return &s }
func ptrBool(b bool) *bool { return &b }
