// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedPost writes a hub whose root page is a post, scheduled, featured and asking for
// confirmation, narrowed to one group of which authorA is a member and in which authorA holds a
// role at the hub.
func seedPost(ctx context.Context, t *testing.T) (hub, page, group shared.ID, at time.Time) {
	t.Helper()
	hub, _, _ = seedSpace(ctx, t)
	page, group = freshID(t), freshID(t)
	at = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)`, []any{group.String(), tenantA.String(), "posts-" + group.String()}},
		{`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)`, []any{tenantA.String(), group.String(), authorA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'MEMBER')`,
			[]any{freshID(t).String(), tenantA.String(), authorA.String(), hub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	publish, featured, confirm := at.Add(time.Hour), at.Add(48*time.Hour), at.Add(72*time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := postgres.NewPageRepository().Insert(ctx, knowledge.Page{
			ID: page, HubID: hub, Title: "Release 4.2", Body: "What changed", OrderKey: "z0",
			CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1,
		}); err != nil {
			return err
		}
		return postgres.NewPostRepository().Insert(ctx, knowledge.Post{
			PageID: page, HubID: hub, Template: knowledge.PostReleaseNote, PublishAt: &publish,
			FeaturedUntil: &featured, Audience: knowledge.AudienceGroups, GroupIDs: []shared.ID{group},
			Discussion: true, Notify: true, ConfirmBy: &confirm,
		})
	}); err != nil {
		t.Fatal(err)
	}
	return hub, page, group, at
}

// A post is read as written, stays off the page tree and the page search, is published, seen,
// confirmed and discussed with reactions (migration 0401).
func TestAPostIsReadAsItWasWritten(t *testing.T) {
	ctx := context.Background()
	hub, page, group, at := seedPost(ctx, t)
	posts, pages := postgres.NewPostRepository(), postgres.NewPageRepository()
	reply, answer := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		post, err := posts.Find(ctx, page)
		if err != nil || post.Template != knowledge.PostReleaseNote || post.Audience != knowledge.AudienceGroups ||
			!slices.Equal(post.GroupIDs, []shared.ID{group}) || post.PublishedAt != nil || post.ConfirmBy == nil {
			t.Errorf("post %+v, %v", post, err)
		}
		if found, err := posts.PostPages(ctx, []shared.ID{page, hub}); err != nil || !found[page] || found[hub] {
			t.Errorf("post pages %v, %v", found, err)
		}
		if tree, _ := pages.List(ctx, hub, true, false, 100); slices.ContainsFunc(tree, func(p knowledge.Page) bool { return p.ID == page }) {
			t.Error("a post stood in the page tree")
		}
		if hits, _ := pages.Search(ctx, repository.PageQuery{Text: "changed", HubIDs: []shared.ID{hub}, Limit: 10}); len(hits) != 0 {
			t.Errorf("a page search found the post: %+v", hits)
		}
		post = post.Published(at.Add(2 * time.Hour))
		post.Audience, post.GroupIDs = knowledge.AudienceHub, nil
		if err := posts.Update(ctx, post); err != nil {
			return err
		}
		if err := posts.MarkSeen(ctx, page, authorA, at); err != nil {
			return err
		}
		if err := posts.MarkSeen(ctx, page, authorA, at); err != nil {
			return err
		}
		if err := posts.Confirm(ctx, page, authorA, at); err != nil {
			return err
		}
		rows, err := posts.List(ctx, hub, authorA, repository.PostFilter{}, at.Add(3*time.Hour), 10)
		if err != nil || len(rows) != 1 || rows[0].SeenCount != 1 || !rows[0].Seen || !rows[0].Confirmed ||
			rows[0].Title != "Release 4.2" || rows[0].Post.PublishedAt == nil || rows[0].Post.Audience != knowledge.AudienceHub {
			t.Errorf("rows %+v, %v", rows, err)
		}
		if unread, _ := posts.List(ctx, hub, authorA, repository.PostFilter{Unread: true}, at, 10); len(unread) != 0 {
			t.Errorf("a seen post stayed unread: %+v", unread)
		}
		if mine, _ := posts.List(ctx, hub, authorB, repository.PostFilter{Mine: true}, at, 10); len(mine) != 0 {
			t.Errorf("another's post was listed as mine: %+v", mine)
		}
		if who, err := posts.ConfirmedBy(ctx, page); err != nil || !slices.Equal(who, []shared.ID{authorA}) {
			t.Errorf("confirmed by %v, %v", who, err)
		}
		if people, err := posts.PeopleReaching(ctx, hub, 100); err != nil || !slices.Contains(people, authorA) {
			t.Errorf("people reaching %v, %v", people, err)
		}
		if groups, err := posts.GroupsOf(ctx, []shared.ID{authorA}); err != nil || !slices.Contains(groups[authorA], group) {
			t.Errorf("groups %v, %v", groups, err)
		}
		for _, r := range []repository.Reply{
			{ID: reply, PageID: page, AuthorID: authorA, Body: "Great", CreatedAt: at},
			{ID: answer, PageID: page, ParentID: reply, AuthorID: authorA, Body: "Indeed", CreatedAt: at.Add(time.Minute)},
		} {
			if err := posts.InsertReply(ctx, r); err != nil {
				return err
			}
		}
		if err := posts.React(ctx, reply, authorA, knowledge.ReactionThanks); err != nil {
			return err
		}
		if err := posts.React(ctx, reply, authorA, knowledge.ReactionThanks); err != nil {
			return err
		}
		reactions, err := posts.Reactions(ctx, page, authorA)
		if err != nil || len(reactions) != 1 || reactions[0].Count != 1 || !reactions[0].Mine || reactions[0].ReplyID != reply {
			t.Errorf("reactions %+v, %v", reactions, err)
		}
		if err := posts.Unreact(ctx, reply, authorA, knowledge.ReactionThanks); err != nil {
			return err
		}
		if err := posts.TakeBackReply(ctx, reply, at); err != nil {
			return err
		}
		replies, err := posts.Replies(ctx, page, 10)
		if err != nil || len(replies) != 2 || replies[0].Body != "" || replies[0].DeletedAt == nil || replies[1].ParentID != reply {
			t.Errorf("replies %+v, %v", replies, err)
		}
		if found, err := posts.FindReply(ctx, answer); err != nil || found.Body != "Indeed" {
			t.Errorf("reply %+v, %v", found, err)
		}
		// What a list shows of it: its page's opening, length, labels and cover, its live replies.
		if err := pages.SetLabels(ctx, page, []string{"release", "app"}); err != nil {
			return err
		}
		listed, err := posts.List(ctx, hub, authorA, repository.PostFilter{}, at.Add(3*time.Hour), 10)
		if err != nil || len(listed) != 1 || listed[0].Lead != "What changed" || listed[0].Words != 2 ||
			!slices.Equal(listed[0].Labels, []string{"app", "release"}) || listed[0].Replies != 1 || listed[0].Cover != nil {
			t.Errorf("listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another workspace finds no post, no reader, no confirmation, no reply and no reaction, and
// changes nothing.
func TestAPostIsInvisibleToTheWorkspaceNextDoor(t *testing.T) {
	ctx := context.Background()
	hub, page, _, at := seedPost(ctx, t)
	posts := postgres.NewPostRepository()
	reply := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return posts.InsertReply(ctx, repository.Reply{ID: reply, PageID: page, AuthorID: authorA, Body: "x", CreatedAt: at})
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := posts.Find(ctx, page); err == nil {
			t.Error("the workspace next door found the post")
		}
		if _, err := posts.FindForUpdate(ctx, page); err == nil {
			t.Error("the workspace next door locked the post")
		}
		if err := posts.Update(ctx, knowledge.Post{PageID: page, HubID: hub, Audience: knowledge.AudienceHub}); err == nil {
			t.Error("the workspace next door wrote the post")
		}
		if found, _ := posts.PostPages(ctx, []shared.ID{page}); found[page] {
			t.Error("the workspace next door learnt the page is a post")
		}
		if rows, _ := posts.List(ctx, hub, authorB, repository.PostFilter{}, at, 10); len(rows) != 0 {
			t.Error("the workspace next door listed the posts")
		}
		if who, _ := posts.ConfirmedBy(ctx, page); len(who) != 0 {
			t.Error("the workspace next door read the confirmations")
		}
		if people, _ := posts.PeopleReaching(ctx, hub, 100); slices.Contains(people, authorA) {
			t.Error("the workspace next door read the hub's people")
		}
		if groups, _ := posts.GroupsOf(ctx, []shared.ID{authorA}); len(groups) != 0 {
			t.Error("the workspace next door read the groups")
		}
		if _, err := posts.FindReply(ctx, reply); err == nil {
			t.Error("the workspace next door found the reply")
		}
		if replies, _ := posts.Replies(ctx, page, 10); len(replies) != 0 {
			t.Error("the workspace next door read the discussion")
		}
		if err := posts.TakeBackReply(ctx, reply, at); err == nil {
			t.Error("the workspace next door took the reply back")
		}
		if reactions, _ := posts.Reactions(ctx, page, authorB); len(reactions) != 0 {
			t.Error("the workspace next door read the reactions")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Writing into another workspace's post is refused by its keys and the row policy, each write
	// in a transaction of its own.
	for name, refused := range map[string]func(context.Context) error{
		"marked the post seen": func(ctx context.Context) error { return posts.MarkSeen(ctx, page, authorB, at) },
		"confirmed the post":   func(ctx context.Context) error { return posts.Confirm(ctx, page, authorB, at) },
		"reacted":              func(ctx context.Context) error { return posts.React(ctx, reply, authorB, knowledge.ReactionIdea) },
		"replied": func(ctx context.Context) error {
			return posts.InsertReply(ctx, repository.Reply{ID: freshID(t), PageID: page, AuthorID: authorB, Body: "x", CreatedAt: at})
		},
	} {
		if err := write(ctx, t, tenantB, refused); err == nil {
			t.Errorf("the workspace next door %s", name)
		}
	}
}

// UC-DOC-18 check 6: a person's erasure removes which posts they saw and confirmed and the reactions
// they gave, and lists their replies as POST_REPLY with the post's hub; an unanswered reply is
// removed, an answered one blanked so the answer keeps its thread. Another workspace removes nothing.
func TestAPersonsPostTracesAreErased(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, page, _, at := seedPost(ctx, t)
	subject := seedAccount(ctx, t, tenantA)
	lone, answered, answer := freshID(t), freshID(t), freshID(t)
	posts := postgres.NewPostRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := posts.MarkSeen(ctx, page, subject, at); err != nil {
			return err
		}
		if err := posts.Confirm(ctx, page, subject, at); err != nil {
			return err
		}
		if err := posts.Follow(ctx, hub, subject, at); err != nil {
			return err
		}
		for _, r := range []repository.Reply{
			{ID: lone, PageID: page, AuthorID: subject, Body: "Thanks", CreatedAt: at},
			{ID: answered, PageID: page, AuthorID: subject, Body: "A question", CreatedAt: at},
			{ID: answer, PageID: page, ParentID: answered, AuthorID: authorA, Body: "An answer", CreatedAt: at.Add(time.Minute)},
		} {
			if err := posts.InsertReply(ctx, r); err != nil {
				return err
			}
		}
		return posts.React(ctx, answer, subject, knowledge.ReactionIdea)
	}); err != nil {
		t.Fatal(err)
	}
	traces := func() int {
		return countIn(ctx, t, `SELECT (SELECT count(*) FROM post_read WHERE account_id = $1)
			+ (SELECT count(*) FROM post_confirmation WHERE account_id = $1)
			+ (SELECT count(*) FROM post_reaction WHERE account_id = $1)
			+ (SELECT count(*) FROM post_follow WHERE account_id = $1)`, subject.String())
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := privacyRepo().DiscardArrangements(ctx, subject)
		return err
	}); err == nil && traces() != 4 {
		t.Fatalf("another workspace removed the traces: %d left", traces())
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		contributions, err := privacyRepo().Contributions(ctx, subject)
		if err != nil {
			return err
		}
		var replies []shared.ID
		for _, c := range contributions {
			if c.Kind == privacyrepo.ContributedPostReply && c.HubID == hub {
				replies = append(replies, c.ID)
			}
		}
		if len(replies) != 2 || !slices.Contains(replies, lone) || !slices.Contains(replies, answered) {
			t.Errorf("replies listed %v, want both with the post's hub", replies)
		}
		if _, err := privacyRepo().DiscardArrangements(ctx, subject); err != nil {
			return err
		}
		gone, blanked, err := privacyRepo().DeleteWritings(ctx, subject, privacyrepo.ContributedPostReply, replies, at)
		if len(gone) != 1 || gone[0] != lone || blanked != 1 {
			t.Errorf("removed %v and blanked %d, want the lone reply removed and the answered one blanked", gone, blanked)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if left := traces(); left != 0 {
		t.Errorf("%d traces left after the erasure", left)
	}
	if countIn(ctx, t, `SELECT count(*) FROM post_reply WHERE id = $1 AND body = '' AND author_id IS NULL AND deleted_at IS NOT NULL`, answered.String()) != 1 ||
		countIn(ctx, t, `SELECT count(*) FROM post_reply WHERE id = $1 AND body = 'An answer'`, answer.String()) != 1 {
		t.Error("the answered reply was not blanked, or the answer went with it")
	}
}

// New for you reads every hub's published posts the account has not seen (UC-DOC-18 check 5): a
// scheduled one waits for its moment, a seen one drops out, and the workspace next door lists none.
func TestTheUnreadPostsAreReadAcrossHubs(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, page, _, at := seedPost(ctx, t)
	reader := seedAccount(ctx, t, tenantA)
	posts := postgres.NewPostRepository()
	unread := func(tenant shared.ID, now time.Time) []shared.ID {
		var ids []shared.ID
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			rows, err := posts.List(ctx, "", reader, repository.PostFilter{Unread: true, Published: true}, now, 50)
			for _, row := range rows {
				ids = append(ids, row.Post.PageID)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if slices.Contains(unread(tenantA, at), page) {
		t.Error("a scheduled post was listed before its moment")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		post, err := posts.FindForUpdate(ctx, page)
		if err != nil {
			return err
		}
		return posts.Update(ctx, post.Published(at.Add(time.Hour)))
	}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(2 * time.Hour)
	if !slices.Contains(unread(tenantA, later), page) {
		t.Error("a published post the account has not seen is missing")
	}
	if slices.Contains(unread(tenantB, later), page) {
		t.Error("the workspace next door listed the post")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return posts.MarkSeen(ctx, page, reader, later) }); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(unread(tenantA, later), page) {
		t.Error("a seen post is still new")
	}
}

// Following a hub's posts (B-5): set, answered and listed in its own workspace, forgotten on
// unfollowing; the workspace next door neither sees nor changes it (rule 3).
func TestFollowingAHubsPostsStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, _, _, at := seedPost(ctx, t)
	follower := seedAccount(ctx, t, tenantA)
	posts := postgres.NewPostRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := posts.Follow(ctx, hub, follower, at); err != nil {
			return err
		}
		return posts.Follow(ctx, hub, follower, at.Add(time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		follows, err := posts.Follows(ctx, hub, follower)
		if err != nil || !follows {
			t.Errorf("follows %v, %v; wants following", follows, err)
		}
		followers, err := posts.Followers(ctx, hub, 100)
		if err != nil || !slices.Equal(followers, []shared.ID{follower}) {
			t.Errorf("followers %v, %v; wants the follower once", followers, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if follows, err := posts.Follows(ctx, hub, follower); err != nil || follows {
			t.Errorf("next door answers following %v, %v", follows, err)
		}
		if followers, err := posts.Followers(ctx, hub, 100); err != nil || len(followers) != 0 {
			t.Errorf("next door lists followers %v, %v", followers, err)
		}
		if err := posts.Unfollow(ctx, hub, follower); err != nil {
			return err
		}
		if err := posts.Follow(ctx, hub, follower, at); err == nil {
			t.Error("next door followed a hub it does not hold")
		}
		return nil
	}); err != nil {
		t.Logf("next door's write refused: %v", err)
	}
	if countIn(ctx, t, `SELECT count(*) FROM post_follow WHERE hub_id = $1`, hub.String()) != 1 {
		t.Fatal("next door changed who follows the hub")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return posts.Unfollow(ctx, hub, follower)
	}); err != nil {
		t.Fatal(err)
	}
	if countIn(ctx, t, `SELECT count(*) FROM post_follow WHERE hub_id = $1`, hub.String()) != 0 {
		t.Error("unfollowing left the row")
	}
}

// A post is found by its words once it has gone out - neither as a draft nor before its moment -
// with the passage that matched, in its hub and across hubs; another hub and the workspace next
// door find nothing (rule 3).
func TestAPostIsFoundByItsWordsOnceItHasGoneOut(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, page, _, at := seedPost(ctx, t)
	posts := postgres.NewPostRepository()
	found := func(tenant, hubID shared.ID, words string, now time.Time) []repository.PostHit {
		var hits []repository.PostHit
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			hits, err = posts.Search(ctx, words, hubID, now, 50)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(hits, func(h repository.PostHit) bool { return h.Post.PageID != page })
	}
	if hits := found(tenantA, hub, "changed", at); len(hits) != 0 {
		t.Errorf("a scheduled post was found before its moment: %+v", hits)
	}
	published := at.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		post, err := posts.FindForUpdate(ctx, page)
		if err != nil {
			return err
		}
		return posts.Update(ctx, post.Published(published))
	}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(2 * time.Hour)
	hits := found(tenantA, hub, "changed", later)
	if len(hits) != 1 || hits[0].Title != "Release 4.2" || hits[0].CreatedBy != authorA || hits[0].Excerpt != "What «changed»" ||
		hits[0].Post.PublishedAt == nil || !hits[0].Post.PublishedAt.Equal(published) || hits[0].Post.HubID != hub {
		t.Errorf("hits %+v", hits)
	}
	if hits := found(tenantA, "", "release", later); len(hits) != 1 {
		t.Errorf("a search across hubs found %+v", hits)
	}
	if hits := found(tenantA, freshID(t), "changed", later); len(hits) != 0 {
		t.Errorf("another hub found the post: %+v", hits)
	}
	if hits := found(tenantA, hub, "unrelated", later); len(hits) != 0 {
		t.Errorf("words the post does not hold found it: %+v", hits)
	}
	if hits := found(tenantB, "", "changed", later); len(hits) != 0 {
		t.Errorf("the workspace next door found the post: %+v", hits)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		post, err := posts.FindForUpdate(ctx, page)
		if err != nil {
			return err
		}
		post.PublishAt, post.PublishedAt = nil, nil
		return posts.Update(ctx, post)
	}); err != nil {
		t.Fatal(err)
	}
	if hits := found(tenantA, hub, "changed", later); len(hits) != 0 {
		t.Errorf("a draft was found: %+v", hits)
	}
}
