// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-000000000011")
	hubID        = shared.MustParseID("0192f000-0000-7000-8000-000000000012")
	itemID       = shared.MustParseID("0192f000-0000-7000-8000-000000000013")
)

// --- the ports these two use cases add, as fakes -------------------------------------------

// containers answers Find and nothing else. The rest of the port is here because Go needs it, not
// because a media use case has any business with a container beyond the path above it.
type containers struct {
	stored map[shared.ID]work.Container
}

func newContainers() *containers {
	return &containers{stored: map[shared.ID]work.Container{
		collectionID: {
			ID: collectionID, TenantID: tenantID, ParentID: hubID,
			Type: work.ContainerCollection,
		},
	}}
}

func (c *containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, ok := c.stored[id]
	if !ok {
		return work.Container{}, shared.ErrNotFound
	}
	return container, nil
}

func (c *containers) List(context.Context, workrepo.ContainerQuery) (workrepo.ContainerPage, error) {
	return workrepo.ContainerPage{}, nil
}
func (c *containers) LastOrderKey(context.Context, shared.ID) (string, error) { return "", nil }
func (c *containers) Insert(context.Context, work.Container) error            { return nil }
func (c *containers) SetAttributes(context.Context, work.Container, int) error {
	return nil
}
func (c *containers) SetPolicies(context.Context, work.Container, int) error { return nil }
func (c *containers) SetArchived(context.Context, work.Container, int) error { return nil }
func (c *containers) TrashSubtree(context.Context, workrepo.ContainerTrash) (workrepo.Cascade, error) {
	return workrepo.Cascade{}, nil
}
func (c *containers) RestoreBatch(context.Context, workrepo.ContainerTrash) (workrepo.Cascade, error) {
	return workrepo.Cascade{}, nil
}
func (c *containers) SetPlacement(context.Context, work.Container, int) error { return nil }
func (c *containers) Neighbours(context.Context, shared.ID, shared.ID, shared.ID) (string, string, error) {
	return "", "", nil
}

// reader is the many-paths permission answer, recorded so a test can say how often it was asked.
type reader struct {
	allow bool
	asked [][][]identity.Scope
	err   error
}

func (r *reader) Permitted(
	_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope,
) ([]bool, error) {
	r.asked = append(r.asked, paths)
	if r.err != nil {
		return nil, r.err
	}
	allowed := make([]bool, len(paths))
	for i := range allowed {
		allowed[i] = r.allow
	}
	return allowed, nil
}

// authorizer is the single-path answer DeleteMedia asks about an administrator.
type authorizer struct {
	err      error
	requests []access.Request
}

func (a *authorizer) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.err
}

// --- reading ---------------------------------------------------------------------------------

func readingHarness() (GetMedia, *objects, *reader, *transfers) {
	records, permission, targets := newObjects(), &reader{}, &transfers{}
	return GetMedia{
		Objects:    records,
		Containers: newContainers(),
		Transfers:  targets,
		Reader:     permission,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
	}, records, permission, targets
}

// ready is one sealed object of this tenant, staged by the account the actor helper names.
func ready(uploader shared.ID) domain.Object {
	return domain.Object{
		ID: mintedID, TenantID: tenantID,
		StorageKey: "media/" + tenantID.String() + "/" + mintedID.String(),
		FileName:   "plan.png", ContentType: "image/png", ByteSize: 32,
		Usage: domain.UsageAttachment, Status: domain.StatusReady,
		CreatedBy: uploader, CreatedAt: now,
	}
}

func TestTheUploaderReadsTheirObjectAndItsDownloadTarget(t *testing.T) {
	handler, records, permission, targets := readingHarness()
	records.stored[mintedID] = ready(accountID)

	viewed, err := handler.Execute(t.Context(), actor(mediaRead), MediaQuery{MediaID: mintedID})
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}

	if viewed.Object.ID != mintedID {
		t.Errorf("the answer is about %s", viewed.Object.ID)
	}
	if viewed.Download.Method != "GET" {
		t.Errorf("the download target is %+v", viewed.Download)
	}
	if want := now.Add(DownloadWindow); viewed.Download.ExpiresAt != want {
		t.Errorf("the target expires at %v, want %v", viewed.Download.ExpiresAt, want)
	}
	if len(targets.downloads) != 1 {
		t.Errorf("%d download targets minted, want 1", len(targets.downloads))
	}
	// The uploader is not asked about: a staged object sits under nobody's container, so there is
	// no path to resolve a membership along.
	if len(permission.asked) != 0 {
		t.Errorf("the authorisation service was asked %d times", len(permission.asked))
	}
}

func TestAPendingObjectIsAnsweredWithoutADownloadTarget(t *testing.T) {
	handler, records, _, targets := readingHarness()
	staged := ready(accountID)
	staged.Status = domain.StatusPending
	records.stored[mintedID] = staged

	viewed, err := handler.Execute(t.Context(), actor(mediaRead), MediaQuery{MediaID: mintedID})
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}

	// Nothing has judged those bytes, and a capability to fetch them would be exactly the
	// rendering path T-11 forbids.
	if viewed.Download.URL != "" {
		t.Errorf("a PENDING object was handed a download target: %+v", viewed.Download)
	}
	if len(targets.downloads) != 0 {
		t.Error("a target was minted for an unjudged object")
	}
}

func TestSomebodyWhoMayReadAReferencingEntryReadsTheObject(t *testing.T) {
	handler, records, permission, _ := readingHarness()
	records.stored[mintedID] = ready(strangerA)
	records.refs[mintedID] = []repository.ItemRef{
		{ItemID: itemID, CollectionID: collectionID},
	}
	permission.allow = true

	if _, err := handler.Execute(
		t.Context(), actor(mediaRead), MediaQuery{MediaID: mintedID},
	); err != nil {
		t.Fatalf("reading failed: %v", err)
	}

	if len(permission.asked) != 1 || len(permission.asked[0]) != 1 {
		t.Fatalf("the authorisation service was asked %v", permission.asked)
	}
	// The path runs from the tenant down through the hub to the collection, so a membership held
	// anywhere above the entry counts (domain-model.md §3.2).
	want := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
	if got := permission.asked[0][0]; !samePath(got, want) {
		t.Errorf("the path asked about is %v, want %v", got, want)
	}
}

func TestAnObjectNobodyMayReachIsReportedMissing(t *testing.T) {
	cases := []struct {
		name string
		set  func(*objects, *reader)
	}{
		{
			// Somebody else's staging, attached to nothing: there is no entry to be allowed to
			// read, so there is nothing that could make it readable.
			name: "staged by somebody else and referenced by nothing",
			set:  func(*objects, *reader) {},
		},
		{
			name: "referenced by an entry the actor may not read",
			set: func(records *objects, permission *reader) {
				records.refs[mintedID] = []repository.ItemRef{
					{ItemID: itemID, CollectionID: collectionID},
				}
				permission.allow = false
			},
		},
		{
			name: "already marked for the reconciliation to remove",
			set: func(records *objects, _ *reader) {
				marked := ready(accountID)
				marked.DeletedAt = &now
				records.stored[mintedID] = marked
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handler, records, permission, _ := readingHarness()
			records.stored[mintedID] = ready(strangerA)
			c.set(records, permission)

			_, err := handler.Execute(t.Context(), actor(mediaRead), MediaQuery{MediaID: mintedID})
			// Missing rather than forbidden: two spellings would be an oracle for which
			// identifiers exist (T-04).
			if !errors.Is(err, shared.ErrNotFound) {
				t.Fatalf("error %v, want not found", err)
			}
		})
	}
}

func TestReadingNeedsTheReadScope(t *testing.T) {
	handler, records, _, _ := readingHarness()
	records.stored[mintedID] = ready(accountID)

	_, err := handler.Execute(t.Context(), actor("items:read"), MediaQuery{MediaID: mintedID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
}

// --- removing --------------------------------------------------------------------------------

func removalHarness() (DeleteMedia, *objects, *authorizer, *sink) {
	records, permission, trail := newObjects(), &authorizer{}, &sink{}
	return DeleteMedia{
		Objects:    records,
		Authorizer: permission,
		Audit:      trail,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
	}, records, permission, trail
}

func TestTheUploaderRemovesAnUnreferencedObject(t *testing.T) {
	handler, records, permission, trail := removalHarness()
	records.stored[mintedID] = ready(accountID)

	if err := handler.Execute(
		t.Context(), actor(mediaWrite), DeleteCommand{MediaID: mintedID},
	); err != nil {
		t.Fatalf("removal failed: %v", err)
	}

	if len(records.marked) != 1 || records.marked[0] != mintedID {
		t.Errorf("the object was not marked: %v", records.marked)
	}
	// Marked, not gone: the bytes and the row go with the reconciliation job, which is what writes
	// the deletion journal entry (data-protection.md §5).
	if records.stored[mintedID].DeletedAt == nil {
		t.Error("the record was removed rather than marked")
	}
	if len(permission.requests) != 0 {
		t.Error("the uploader was put through an authorisation question")
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != MediaDeletedAction {
		t.Fatalf("the removal was not recorded: %+v", trail.entries)
	}
	if _, named := trail.entries[0].Changes["file_name"]; named {
		t.Error("the file name reached the audit trail")
	}
}

func TestARemovalIsRefusedWhileAnythingReferencesTheObject(t *testing.T) {
	handler, records, _, trail := removalHarness()
	records.stored[mintedID] = ready(accountID)
	records.referenced[mintedID] = true

	err := handler.Execute(t.Context(), actor(mediaWrite), DeleteCommand{MediaID: mintedID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
	if len(trail.entries) != 0 {
		t.Error("a refused removal was recorded as a success")
	}
}

func TestSomebodyElsesObjectIsPutThroughTheAdministratorQuestion(t *testing.T) {
	handler, records, permission, _ := removalHarness()
	records.stored[mintedID] = ready(strangerA)

	if err := handler.Execute(
		t.Context(), actor(mediaWrite), DeleteCommand{MediaID: mintedID},
	); err != nil {
		t.Fatalf("removal failed: %v", err)
	}

	if len(permission.requests) != 1 {
		t.Fatalf("the authorisation service was asked %d times", len(permission.requests))
	}
	// At the tenant scope: a role held on one hub says nothing about a file that may be attached
	// to entries in another.
	asked := permission.requests[0]
	if len(asked.Path) != 1 || asked.Path[0].Type != identity.ScopeTenant {
		t.Errorf("the path asked about is %v", asked.Path)
	}
}

func TestARefusedAdministratorRemovesNothing(t *testing.T) {
	handler, records, permission, trail := removalHarness()
	records.stored[mintedID] = ready(strangerA)
	permission.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	err := handler.Execute(t.Context(), actor(mediaWrite), DeleteCommand{MediaID: mintedID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(records.marked) != 0 {
		t.Error("the object was marked despite the refusal")
	}
	// The refusal's own entry is the authorisation service's to write; this one records successes.
	if len(trail.entries) != 0 {
		t.Errorf("a refusal was recorded as a removal: %+v", trail.entries)
	}
}

func TestRemovingAnAlreadyMarkedObjectIsReportedMissing(t *testing.T) {
	handler, records, _, _ := removalHarness()
	marked := ready(accountID)
	marked.DeletedAt = &now
	records.stored[mintedID] = marked

	err := handler.Execute(t.Context(), actor(mediaWrite), DeleteCommand{MediaID: mintedID})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}

func samePath(got, want []identity.Scope) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// --- what an entry carries ---------------------------------------------------------------------

// workItems answers Find and nothing else. The rest of the port is here because Go needs it, not
// because a media use case has any business writing an entry.
type workItems struct {
	stored map[shared.ID]work.WorkItem
}

func newWorkItems() *workItems {
	return &workItems{stored: map[shared.ID]work.WorkItem{
		itemID: {
			ID: itemID, TenantID: tenantID, CollectionID: collectionID, Type: work.ItemTask,
			Path: work.RootPath(itemID), Depth: 1, Title: "File the expenses", Version: 1,
		},
	}}
}

func (i *workItems) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, ok := i.stored[id]
	if !ok {
		return work.WorkItem{}, shared.ErrNotFound
	}
	return item, nil
}

func (i *workItems) List(context.Context, workrepo.ItemQuery) (workrepo.ItemPage, error) {
	return workrepo.ItemPage{}, nil
}
func (i *workItems) ChildCompletion(context.Context, shared.ID) (work.ChildCompletion, error) {
	return work.ChildCompletion{}, nil
}
func (i *workItems) SetCompletion(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) SetAttributes(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) Neighbours(
	context.Context, workrepo.Level, shared.ID, shared.ID,
) (string, string, error) {
	return "", "", nil
}
func (i *workItems) SetOrderKey(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) SetAssignee(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) CountOpenByAssignee(
	context.Context, []shared.ID,
) (map[shared.ID]int, error) {
	return nil, nil
}
func (i *workItems) SetCustomFields(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) SetCover(context.Context, work.WorkItem, int) error        { return nil }
func (i *workItems) MoveSubtree(
	context.Context, workrepo.Move,
) (int, []work.DroppedReference, error) {
	return 0, nil, nil
}
func (i *workItems) LastOrderKey(context.Context, shared.ID, shared.ID) (string, error) {
	return "", nil
}
func (i *workItems) Insert(context.Context, work.WorkItem) error           { return nil }
func (i *workItems) SetArchived(context.Context, work.WorkItem, int) error { return nil }
func (i *workItems) TrashSubtree(context.Context, workrepo.ItemTrash) (int, error) {
	return 0, nil
}
func (i *workItems) RestoreBatch(context.Context, workrepo.ItemTrash) (int, error) {
	return 0, nil
}
func (i *workItems) Query(
	context.Context, workrepo.ItemSearch,
) (workrepo.ItemQueryResult, error) {
	return workrepo.ItemQueryResult{}, nil
}

func listingHarness() (ListAttachments, *objects, *authorizer) {
	records, permission := newObjects(), &authorizer{}
	return ListAttachments{
		Objects:    records,
		Items:      newWorkItems(),
		Containers: newContainers(),
		Authorizer: permission,
		UnitOfWork: &unitOfWork{},
	}, records, permission
}

func TestTheAttachmentsOfAnEntryComeBackAsMediaRecords(t *testing.T) {
	handler, records, _ := listingHarness()
	records.pages[itemID] = repository.ObjectPage{Objects: []domain.Object{ready(accountID)}}

	page, err := handler.Execute(t.Context(), actor(mediaRead), AttachmentsQuery{ItemID: itemID})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}

	if len(page.Objects) != 1 || page.Objects[0].ID != mintedID {
		t.Fatalf("the page is %+v", page.Objects)
	}
	// Clamped rather than refused: a client asking for more than the contract allows wants as many
	// as it can have.
	if records.asked[itemID].Size != defaultPageSize {
		t.Errorf("the page size asked for is %d, want %d", records.asked[itemID].Size, defaultPageSize)
	}
}

func TestAnOversizedPageIsClamped(t *testing.T) {
	handler, records, _ := listingHarness()

	if _, err := handler.Execute(t.Context(), actor(mediaRead), AttachmentsQuery{
		ItemID: itemID, Size: 5000,
	}); err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if records.asked[itemID].Size != maxPageSize {
		t.Errorf("the page size asked for is %d, want %d", records.asked[itemID].Size, maxPageSize)
	}
}

func TestListingRefusesSomebodyWhoMayNotReadTheEntry(t *testing.T) {
	handler, records, permission := listingHarness()
	permission.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := handler.Execute(t.Context(), actor(mediaRead), AttachmentsQuery{ItemID: itemID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	// Refused before the read: a client that may not see the entry does not pay for the query, and
	// this server does not run one for it.
	if _, asked := records.asked[itemID]; asked {
		t.Error("the attachments were read despite the refusal")
	}
}

func TestListingAnEntryThatIsNotThereIsReportedMissing(t *testing.T) {
	handler, _, _ := listingHarness()

	_, err := handler.Execute(t.Context(), actor(mediaRead), AttachmentsQuery{ItemID: strangerA})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
}
