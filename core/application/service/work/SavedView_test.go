// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	savedViewID = shared.MustParseID("0192f000-0000-7000-8000-000000000081")
	otherOwner  = shared.MustParseID("0192f000-0000-7000-8000-000000000082")
)

// savedViews is the port's fake: the same optimistic lock the statements keep, so a use case that
// passed the wrong version would look wrong at this level already.
type savedViews struct {
	stored map[shared.ID]view.SavedView
	// reachAsked records the scope identifiers ListReachable was handed, so a test can say the
	// authorisation's answer reached the statement.
	reachAsked [][]shared.ID
}

func (s *savedViews) Find(_ context.Context, id shared.ID) (view.SavedView, error) {
	found, ok := s.stored[id]
	if !ok {
		return view.SavedView{}, shared.ErrNotFound.WithDetail("views.not_found")
	}
	return found, nil
}

func (s *savedViews) ListOwned(_ context.Context, ownerID shared.ID) ([]view.SavedView, error) {
	var views []view.SavedView
	for _, saved := range s.stored {
		if saved.OwnerID == ownerID {
			views = append(views, saved)
		}
	}
	return views, nil
}

func (s *savedViews) ListReachable(
	_ context.Context, ownerID shared.ID, scopeIDs []shared.ID,
) ([]view.SavedView, error) {
	s.reachAsked = append(s.reachAsked, scopeIDs)
	inScope := map[shared.ID]bool{}
	for _, id := range scopeIDs {
		inScope[id] = true
	}
	var views []view.SavedView
	for _, saved := range s.stored {
		published := saved.Sharing == view.SharingScope &&
			(saved.ScopeType == view.ViewScopeTenant || inScope[saved.ScopeID])
		if saved.OwnerID == ownerID || published {
			views = append(views, saved)
		}
	}
	return views, nil
}

func (s *savedViews) Insert(_ context.Context, saved view.SavedView) error {
	s.stored[saved.ID] = saved
	return nil
}

func (s *savedViews) SetAttributes(_ context.Context, saved view.SavedView, expectedVersion int) error {
	return s.write(saved, expectedVersion)
}

func (s *savedViews) SetSharing(_ context.Context, saved view.SavedView, expectedVersion int) error {
	return s.write(saved, expectedVersion)
}

func (s *savedViews) Delete(_ context.Context, saved view.SavedView, expectedVersion int) error {
	if s.stored[saved.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("views.version_conflict")
	}
	delete(s.stored, saved.ID)
	return nil
}

func (s *savedViews) write(saved view.SavedView, expectedVersion int) error {
	if s.stored[saved.ID].Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("views.version_conflict")
	}
	written := saved
	written.Version = expectedVersion + 1
	s.stored[saved.ID] = written
	return nil
}

// permitting is the silent half of the authorisation service: what Permits answers, and what it
// was asked about.
type permitting struct {
	allow bool
	asked []access.Request
}

func (p *permitting) Permits(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) (bool, error) {
	p.asked = append(p.asked, request)
	return p.allow, nil
}

type savedViewHarness struct {
	create     CreateSavedView
	list       ListSavedViews
	get        GetSavedView
	update     UpdateSavedView
	deleteView DeleteSavedView
	share      ShareSavedView

	views      *savedViews
	containers *containers
	authorizer *authorizer
	permits    *permitting
	audit      *sink
	uow        *unitOfWork
}

func newSavedViewHarness(t *testing.T) *savedViewHarness {
	t.Helper()

	h := &savedViewHarness{
		views:      &savedViews{stored: map[shared.ID]view.SavedView{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		authorizer: &authorizer{},
		permits:    &permitting{},
		audit:      &sink{},
		uow:        &unitOfWork{},
	}

	h.create = CreateSavedView{
		Views: h.views, Containers: h.containers, Authorizer: h.authorizer,
		Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{},
	}
	h.list = ListSavedViews{
		Views: h.views, Containers: h.containers, Authorizer: h.authorizer, UnitOfWork: h.uow,
	}
	h.get = GetSavedView{
		Views: h.views, Containers: h.containers, Permits: h.permits, UnitOfWork: h.uow,
	}
	writer := SavedViewWriter{
		Views: h.views, Containers: h.containers, Authorizer: h.authorizer,
		Permits: h.permits, Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now),
	}
	h.update = UpdateSavedView{Writer: writer}
	h.deleteView = DeleteSavedView{Writer: writer}
	h.share = ShareSavedView{Writer: writer}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

func (h *savedViewHarness) withView(owner shared.ID, sharing view.Sharing) view.SavedView {
	saved, err := view.NewSavedView(view.NewSavedViewInput{
		ID: savedViewID, TenantID: tenantID, OwnerID: owner,
		ScopeType: view.ViewScopeCollection, ScopeID: collectionID,
		Name: "Due this week", Layout: "KANBAN",
		Query: map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P7D"},
		},
		Sharing: view.SharingPrivate,
		Now:     now,
	})
	if err != nil {
		panic(err)
	}
	saved.Sharing = sharing
	h.views.stored[saved.ID] = saved
	return saved
}

func viewActor() appshared.ActorContext {
	built := actor()
	built.Scopes = []string{"containers:write", "containers:read"}
	return built
}

func createViewCmd() CreateSavedViewCommand {
	return CreateSavedViewCommand{
		ScopeType: view.ViewScopeCollection, ScopeID: collectionID,
		Name: "Due this week", Layout: "KANBAN",
		Query: map[string]any{
			"filter": map[string]any{"field": "due_at", "op": "LTE", "value": "@today+P7D"},
		},
		Sharing: view.SharingPrivate,
	}
}

// A view is a bookmark: creating one asks READ on its scope's path, and creating it already
// shared asks STRUCTURE - the same permission :share asks.
func TestCreatingAViewAsksTheScopesPermission(t *testing.T) {
	h := newSavedViewHarness(t)

	created, err := h.create.Execute(context.Background(), viewActor(), createViewCmd())
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if created.OwnerID != accountID || created.Version != 1 {
		t.Errorf("the view came out as %+v", created)
	}
	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].Permission != service.PermissionRead {
		t.Fatalf("the permission question was %+v", h.authorizer.requests)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ViewCreatedAction {
		t.Errorf("the audit trail holds %+v", h.audit.entries)
	}

	cmd := createViewCmd()
	cmd.Sharing = view.SharingScope
	if _, err := h.create.Execute(context.Background(), viewActor(), cmd); err != nil {
		t.Fatalf("creating shared failed: %v", err)
	}
	if last := h.authorizer.requests[len(h.authorizer.requests)-1]; last.Permission != service.PermissionStructure {
		t.Errorf("creating shared asked %v", last.Permission)
	}
}

// The scope has to be what it claims: a COLLECTION view over a hub would be a lie every reader
// inherits.
func TestAViewScopeMustMatchItsContainer(t *testing.T) {
	h := newSavedViewHarness(t)

	cmd := createViewCmd()
	cmd.ScopeType, cmd.ScopeID = view.ViewScopeHub, collectionID

	_, err := h.create.Execute(context.Background(), viewActor(), cmd)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "views.scope_container_mismatched" {
		t.Fatalf("a mismatched scope answered %v", err)
	}
}

// A personal view is self-service, like one's own preferences: no matrix cell, the token scope as
// the door, and always the caller's own.
func TestAPersonalViewIsSelfService(t *testing.T) {
	h := newSavedViewHarness(t)

	cmd := createViewCmd()
	cmd.ScopeType, cmd.ScopeID = view.ViewScopeAccount, noID

	created, err := h.create.Execute(context.Background(), viewActor(), cmd)
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if created.ScopeID != accountID {
		t.Errorf("the personal view is scoped to %s", created.ScopeID)
	}
	if len(h.authorizer.requests) != 0 {
		t.Errorf("a personal view asked the matrix: %+v", h.authorizer.requests)
	}

	cmd.ScopeID = otherOwner
	if _, err := h.create.Execute(context.Background(), viewActor(), cmd); shared.AsError(err) == nil {
		t.Error("a personal view for somebody else was accepted")
	}

	cmd.ScopeID = noID
	bare := viewActor()
	bare.Scopes = nil
	if _, err := h.create.Execute(context.Background(), bare, cmd); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a token without the scope answered %v", err)
	}
}

// T-04, the reads: another account's private view and one shared into an unreachable scope both
// answer exactly what a missing view answers.
func TestAnInvisibleViewAnswersNotFound(t *testing.T) {
	h := newSavedViewHarness(t)

	private := h.withView(otherOwner, view.SharingPrivate)
	_, err := h.get.Execute(context.Background(), viewActor(), private.ID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("another account's private view answered %v", err)
	}
	if len(h.permits.asked) != 0 {
		t.Errorf("a private view asked the scope: %+v", h.permits.asked)
	}

	sharedView := h.withView(otherOwner, view.SharingScope)
	h.permits.allow = false
	if _, err := h.get.Execute(context.Background(), viewActor(), sharedView.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("an unreachable shared view answered %v", err)
	}

	h.permits.allow = true
	found, err := h.get.Execute(context.Background(), viewActor(), sharedView.ID)
	if err != nil {
		t.Fatalf("a reachable shared view answered %v", err)
	}
	if found.ID != sharedView.ID {
		t.Errorf("the get answered %+v", found)
	}
	if len(h.permits.asked) == 0 || h.permits.asked[len(h.permits.asked)-1].Permission != service.PermissionRead {
		t.Errorf("the visibility question was %+v", h.permits.asked)
	}
}

// The list with a container answers own plus shared along that container's path, and the path's
// identifiers - the collection and its hub - reach the statement as the bound array.
func TestListingWithAContainerPassesThePathsScopes(t *testing.T) {
	h := newSavedViewHarness(t)
	h.withView(otherOwner, view.SharingScope)

	views, err := h.list.Execute(context.Background(), viewActor(),
		ListSavedViewsQuery{ContainerID: collectionID})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(views) != 1 {
		t.Errorf("the list answered %d views", len(views))
	}
	if len(h.views.reachAsked) != 1 || len(h.views.reachAsked[0]) != 2 {
		t.Fatalf("the statement was handed %+v", h.views.reachAsked)
	}
	if len(h.authorizer.requests) != 1 || h.authorizer.requests[0].Permission != service.PermissionRead {
		t.Errorf("the permission question was %+v", h.authorizer.requests)
	}

	// Without a container: the caller's own shelf, no matrix question.
	if _, err := h.list.Execute(context.Background(), viewActor(), ListSavedViewsQuery{}); err != nil {
		t.Fatalf("listing own failed: %v", err)
	}
	if len(h.authorizer.requests) != 1 {
		t.Errorf("listing own asked the matrix: %+v", h.authorizer.requests)
	}
}

// The owner edits their own; anyone else needs STRUCTURE on the view's scope - and somebody who
// cannot even see the view is told it does not exist.
func TestChangingAnotherAccountsViewAsksStructure(t *testing.T) {
	h := newSavedViewHarness(t)
	sharedView := h.withView(otherOwner, view.SharingScope)
	h.permits.allow = true

	rename := view.ViewAttributes{Name: viewName("Overdue")}
	if _, err := h.update.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: sharedView.ID}, rename); err != nil {
		t.Fatalf("the structural edit failed: %v", err)
	}
	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].Permission != service.PermissionStructure {
		t.Fatalf("the permission question was %+v", h.authorizer.requests)
	}

	h.authorizer.err = shared.ErrForbidden
	if _, err := h.update.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: sharedView.ID}, rename); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused structural edit answered %v", err)
	}
	h.authorizer.err = nil

	// The owner's own edit asks no matrix question.
	own := h.views.stored[sharedView.ID]
	own.OwnerID = accountID
	h.views.stored[sharedView.ID] = own
	before := len(h.authorizer.requests)
	if _, err := h.update.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: sharedView.ID}, view.ViewAttributes{Name: viewName("Mine")}); err != nil {
		t.Fatalf("the owner's edit failed: %v", err)
	}
	if len(h.authorizer.requests) != before {
		t.Errorf("the owner's edit asked the matrix")
	}
}

// Publishing asks STRUCTURE whoever asks, the owner included; taking one's own view back to
// PRIVATE asks nobody. Idempotence spends nothing.
func TestSharingPublishesUnderStructure(t *testing.T) {
	h := newSavedViewHarness(t)
	own := h.withView(accountID, view.SharingPrivate)

	published, err := h.share.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: own.ID}, view.SharingScope)
	if err != nil {
		t.Fatalf("sharing failed: %v", err)
	}
	if published.Sharing != view.SharingScope || published.Version != 2 {
		t.Errorf("sharing produced %+v", published)
	}
	if len(h.authorizer.requests) != 1 ||
		h.authorizer.requests[0].Permission != service.PermissionStructure {
		t.Fatalf("publishing asked %+v", h.authorizer.requests)
	}
	if h.audit.entries[len(h.audit.entries)-1].Action != ViewSharedAction {
		t.Errorf("the audit trail holds %+v", h.audit.entries)
	}

	// The same ask again: nothing written, no version spent.
	repeated, err := h.share.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: own.ID}, view.SharingScope)
	if err != nil || repeated.Version != 2 {
		t.Fatalf("the repeat answered %v, version %d", err, repeated.Version)
	}

	// Taking it back is the owner's, without a permission question.
	before := len(h.authorizer.requests)
	unshared, err := h.share.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: own.ID}, view.SharingPrivate)
	if err != nil || unshared.Sharing != view.SharingPrivate {
		t.Fatalf("unsharing answered %v, %+v", err, unshared.Sharing)
	}
	if len(h.authorizer.requests) != before {
		t.Errorf("the owner's unsharing asked the matrix")
	}
}

// Deleting removes the row and records the act; a stale If-Match is refused before anything runs.
func TestDeletingAViewRemovesAndRecords(t *testing.T) {
	h := newSavedViewHarness(t)
	own := h.withView(accountID, view.SharingPrivate)

	stale := SavedViewCommand{ViewID: own.ID, ExpectedVersion: 7}
	if err := h.deleteView.Execute(context.Background(), viewActor(), stale); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale delete answered %v", err)
	}

	if err := h.deleteView.Execute(context.Background(), viewActor(),
		SavedViewCommand{ViewID: own.ID}); err != nil {
		t.Fatalf("deleting failed: %v", err)
	}
	if _, held := h.views.stored[own.ID]; held {
		t.Error("the view survived its deletion")
	}
	if h.audit.entries[len(h.audit.entries)-1].Action != ViewDeletedAction {
		t.Errorf("the audit trail holds %+v", h.audit.entries)
	}
}

// The untyped door: PUBLIC_LINK is refused by name wherever it arrives, and the update refuses an
// empty patch.
func TestTheViewChannelsRefuseWhatTheModelRefuses(t *testing.T) {
	h := newSavedViewHarness(t)
	own := h.withView(accountID, view.SharingPrivate)

	_, err := h.share.Descriptor().Handler.Invoke(context.Background(), viewActor(),
		map[string]any{"view_id": own.ID.String(), "sharing": "PUBLIC_LINK"})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "views.public_link_not_available" {
		t.Fatalf("PUBLIC_LINK answered %v", err)
	}

	_, err = h.update.Descriptor().Handler.Invoke(context.Background(), viewActor(),
		map[string]any{"view_id": own.ID.String()})
	refusal = shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "views.update_empty" {
		t.Fatalf("an empty patch answered %v", err)
	}

	out, err := h.get.Descriptor().Handler.Invoke(context.Background(), viewActor(),
		map[string]any{"view_id": own.ID.String()})
	if err != nil {
		t.Fatalf("the untyped get failed: %v", err)
	}
	if out.String("layout") != "KANBAN" || out.String("sharing") != "PRIVATE" {
		t.Errorf("the output reads %+v", out)
	}
}

func viewName(value string) *string { return &value }
