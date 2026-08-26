// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	feed "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// calendarFeeds is the port's fake. It keeps the two things the statements keep: a feed is found
// by the hash of its token, and a revocation happens once.
type calendarFeeds struct {
	stored map[shared.ID]feed.CalendarFeed
	// byToken is the index the unique hash is in the database. The credential itself is the key
	// here because a test has no pepper; what it stands for is the hash.
	byToken map[string]shared.ID
	// insertErr lets a test say the shelf failed without a database.
	insertErr error
}

func newCalendarFeeds() *calendarFeeds {
	return &calendarFeeds{
		stored:  map[shared.ID]feed.CalendarFeed{},
		byToken: map[string]shared.ID{},
	}
}

func (s *calendarFeeds) Insert(
	_ context.Context, feed feed.CalendarFeed, token feed.FeedToken,
) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.stored[feed.ID] = feed
	s.byToken[token.Secret()] = feed.ID
	return nil
}

func (s *calendarFeeds) FindByToken(
	_ context.Context, token feed.FeedToken,
) (feed.CalendarFeed, error) {
	id, ok := s.byToken[token.Secret()]
	if !ok {
		return feed.CalendarFeed{}, shared.ErrNotFound.WithDetail("calendar.feed_unknown")
	}
	return s.stored[id], nil
}

func (s *calendarFeeds) Find(_ context.Context, id shared.ID) (feed.CalendarFeed, error) {
	found, ok := s.stored[id]
	if !ok {
		return feed.CalendarFeed{}, shared.ErrNotFound.WithDetail("calendar.feed_unknown")
	}
	return found, nil
}

func (s *calendarFeeds) ListForAccount(
	_ context.Context, accountID shared.ID,
) ([]feed.CalendarFeed, error) {
	var feeds []feed.CalendarFeed
	for _, feed := range s.stored {
		if feed.AccountID == accountID {
			feeds = append(feeds, feed)
		}
	}
	return feeds, nil
}

func (s *calendarFeeds) Revoke(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	feed, ok := s.stored[id]
	if !ok {
		return false, nil
	}
	revoked, changed := feed.Revoked(at)
	s.stored[id] = revoked
	return changed, nil
}

type feedHarness struct {
	create CreateCalendarFeed
	list   ListCalendarFeeds
	revoke RevokeCalendarFeed

	feeds      *calendarFeeds
	views      *savedViews
	containers *containers
	permits    *permitting
	audit      *sink
}

func newFeedHarness(t *testing.T) *feedHarness {
	t.Helper()

	h := &feedHarness{
		feeds:      newCalendarFeeds(),
		views:      &savedViews{stored: map[shared.ID]view.SavedView{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		permits:    &permitting{},
		audit:      &sink{},
	}

	writer := CalendarFeedWriter{
		Feeds: h.feeds, Views: h.views, Containers: h.containers, Permits: h.permits,
		Audit: h.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{},
		Entropy: clock.FixedEntropy{Seed: 0x20},
	}
	h.create = CreateCalendarFeed{Writer: writer}
	h.list = ListCalendarFeeds{Writer: writer}
	h.revoke = RevokeCalendarFeed{Writer: writer}

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

// withView puts a view on the shelf, owned by whoever is named.
func (h *feedHarness) withView(owner shared.ID, sharing view.Sharing) view.SavedView {
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

// feedActor holds the read scope every feed operation is judged against - the view's own, since a
// feed grants exactly what reading the view grants.
func feedActor() appshared.ActorContext {
	acting := actor()
	acting.Scopes = []string{containersRead, containersWrite}
	return acting
}

// Minting answers the credential once, writes the hash, and records that it happened.
func TestMintingAFeedAnswersTheTokenAndRecordsIt(t *testing.T) {
	h := newFeedHarness(t)
	saved := h.withView(accountID, view.SharingPrivate)

	minted, err := h.create.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	if minted.Feed.ViewID != saved.ID || minted.Feed.AccountID != accountID {
		t.Errorf("the feed came out as %+v", minted.Feed)
	}
	if !strings.HasPrefix(minted.Token.Secret(), feed.FeedTokenPrefix) {
		t.Errorf("the token is %q", minted.Token.Secret())
	}

	// The token names its own tenant, which is what the public route opens its transaction as.
	parsed, err := feed.ParseFeedToken(minted.Token.Secret())
	if err != nil {
		t.Fatalf("the minted token does not parse: %v", err)
	}
	if parsed.TenantID() != tenantID {
		t.Errorf("the token names tenant %s", parsed.TenantID())
	}

	// And it finds exactly this feed.
	found, err := h.feeds.FindByToken(t.Context(), minted.Token)
	if err != nil || found.ID != minted.Feed.ID {
		t.Errorf("the token found %+v (%v)", found, err)
	}

	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != FeedCreatedAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}
	// Nothing in the entry is the credential: a feed carries no free text at all, and the one
	// value that would matter is never written anywhere.
	if recorded := fmt.Sprintf("%v", h.audit.entries[0].Changes); strings.Contains(
		recorded, feed.FeedTokenPrefix) {
		t.Errorf("the token reached the audit entry: %s", recorded)
	}
}

// A view the caller cannot see answers what a missing one answers (T-04) - and nothing is minted.
func TestAFeedOverAViewTheCallerCannotSeeIsNotFound(t *testing.T) {
	h := newFeedHarness(t)
	h.permits.allow = false
	saved := h.withView(otherOwner, view.SharingScope)

	_, err := h.create.Execute(t.Context(), feedActor(), saved.ID)
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "views.not_found" {
		t.Fatalf("refused as %v", err)
	}
	if len(h.feeds.stored) != 0 {
		t.Error("a feed was minted over an invisible view")
	}

	// A view that is genuinely absent answers the same thing.
	_, err = h.create.Execute(t.Context(), feedActor(), shared.MustParseID(
		"0192f000-0000-7000-8000-0000000000ff"))
	if second := shared.AsError(err); second == nil || second.DetailCode != "views.not_found" {
		t.Errorf("an absent view answered %v, and the two must not be distinguishable", err)
	}
}

// A view shared into a scope the caller may read is one they may subscribe to: the feed grants
// exactly what its owner may already read.
func TestAFeedMayBeMintedOverAViewSharedWithTheCaller(t *testing.T) {
	h := newFeedHarness(t)
	h.permits.allow = true
	saved := h.withView(otherOwner, view.SharingScope)

	if _, err := h.create.Execute(t.Context(), feedActor(), saved.ID); err != nil {
		t.Fatalf("minting over a shared view failed: %v", err)
	}
	if len(h.permits.asked) == 0 {
		t.Error("the visibility question was never asked")
	}
}

// The list is the caller's own, whatever the role.
func TestTheFeedListIsOnlyTheCallersOwn(t *testing.T) {
	h := newFeedHarness(t)
	saved := h.withView(accountID, view.SharingPrivate)

	mine, err := h.create.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	theirs, err := feed.NewCalendarFeed(feed.NewCalendarFeedInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000c9"), TenantID: tenantID,
		AccountID: otherOwner, ViewID: saved.ID, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.feeds.stored[theirs.ID] = theirs

	listed, err := h.list.Execute(t.Context(), feedActor())
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != mine.Feed.ID {
		t.Fatalf("the list is %+v", listed)
	}
}

// Revocation is idempotent, records once, and is refused for somebody else's feed as not found.
func TestRevokingIsIdempotentAndOnlyEverTheCallersOwn(t *testing.T) {
	h := newFeedHarness(t)
	saved := h.withView(accountID, view.SharingPrivate)
	minted, err := h.create.Execute(t.Context(), feedActor(), saved.ID)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}

	if err := h.revoke.Execute(t.Context(), feedActor(), minted.Feed.ID); err != nil {
		t.Fatalf("revoking failed: %v", err)
	}
	revoked, err := h.feeds.Find(t.Context(), minted.Feed.ID)
	if err != nil || !revoked.IsRevoked() {
		t.Fatalf("the feed reads as %+v (%v)", revoked, err)
	}
	if len(h.audit.entries) != 2 || h.audit.entries[1].Action != FeedRevokedAction {
		t.Fatalf("the audit trail is %+v", h.audit.entries)
	}

	// Again: no error, and no second entry saying something happened that did not.
	if err := h.revoke.Execute(t.Context(), feedActor(), minted.Feed.ID); err != nil {
		t.Fatalf("revoking twice failed: %v", err)
	}
	if len(h.audit.entries) != 2 {
		t.Errorf("a repeat wrote another entry: %+v", h.audit.entries)
	}

	// Somebody else's is not found rather than forbidden.
	theirs, err := feed.NewCalendarFeed(feed.NewCalendarFeedInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ca"), TenantID: tenantID,
		AccountID: otherOwner, ViewID: saved.ID, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.feeds.stored[theirs.ID] = theirs

	err = h.revoke.Execute(t.Context(), feedActor(), theirs.ID)
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "calendar.feed_not_found" {
		t.Fatalf("revoking somebody else's answered %v", err)
	}
	if again, _ := h.feeds.Find(t.Context(), theirs.ID); again.IsRevoked() {
		t.Error("somebody else's feed was revoked after all")
	}
}

// An actor with no account - a service account, an automation - has no authorisation to fetch
// with, so there is nothing for a feed to read as.
func TestAFeedNeedsAnAccountToReadAs(t *testing.T) {
	h := newFeedHarness(t)
	saved := h.withView(accountID, view.SharingPrivate)

	machine := feedActor()
	machine.AccountID = ""
	if _, err := h.create.Execute(t.Context(), machine, saved.ID); err == nil {
		t.Error("a feed was minted with nobody to read as")
	}
	if _, err := h.list.Execute(t.Context(), machine); err == nil {
		t.Error("a list was answered to nobody")
	}
}

// The catalogue's door, which is what MCP and an automation rule come through.
func TestTheFeedUseCasesAnswerThroughTheCatalogue(t *testing.T) {
	h := newFeedHarness(t)
	saved := h.withView(accountID, view.SharingPrivate)

	out, err := h.create.Descriptor().Handler.Invoke(t.Context(), feedActor(),
		usecase.Input{"view_id": saved.ID.String()})
	if err != nil {
		t.Fatalf("minting through the catalogue failed: %v", err)
	}
	if !strings.HasPrefix(out.String("token"), feed.FeedTokenPrefix) {
		t.Fatalf("the projection carries no token: %+v", out)
	}
	if out["view_id"] != saved.ID.String() || out["revoked_at"] != nil {
		t.Errorf("the projection is %+v", out)
	}
	feedID := out.String("id")

	listed, err := h.list.Descriptor().Handler.Invoke(t.Context(), feedActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the catalogue failed: %v", err)
	}
	rows, ok := listed["data"].([]usecase.Output)
	if !ok || len(rows) != 1 {
		t.Fatalf("the list came back as %+v", listed["data"])
	}
	// The list never carries the credential - that is what "shown once" means.
	if _, carried := rows[0]["token"]; carried {
		t.Error("the list carries the token")
	}

	if _, err := h.revoke.Descriptor().Handler.Invoke(t.Context(), feedActor(),
		usecase.Input{"feed_id": feedID}); err != nil {
		t.Fatalf("revoking through the catalogue failed: %v", err)
	}

	// And the projection then says when it stopped.
	listed, err = h.list.Descriptor().Handler.Invoke(t.Context(), feedActor(), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ = listed["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["revoked_at"] == nil {
		t.Errorf("the revoked feed reads as %+v", rows)
	}
}

// What the two commands refuse before they touch anything.
func TestTheFeedCommandsRefuseWhatTheyCannotAct(t *testing.T) {
	h := newFeedHarness(t)

	if _, err := h.create.Execute(t.Context(), feedActor(), ""); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "calendar.view_required" {
		t.Errorf("minting without a view answered %v", err)
	}
	if err := h.revoke.Execute(t.Context(), feedActor(), ""); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "calendar.feed_id_required" {
		t.Errorf("revoking nothing answered %v", err)
	}
	if err := h.revoke.Execute(t.Context(), feedActor(), shared.MustParseID(
		"0192f000-0000-7000-8000-0000000000cb")); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "calendar.feed_not_found" {
		t.Errorf("revoking an absent feed answered %v", err)
	}

	// And the token scope, which every channel is held to.
	stranger := feedActor()
	stranger.Scopes = []string{"items:read"}
	if _, err := h.create.Execute(t.Context(), stranger, savedViewID); err == nil {
		t.Error("a token without the scope minted a feed")
	}
}
