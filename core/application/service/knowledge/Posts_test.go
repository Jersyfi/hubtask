// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The people of the posts' matrix (work-surface concept §12 item 3): a writer and a member of the
// workspace, a guest of the hub alone, a requester who reads nothing, a service account, and an
// agent acting for the member.
var (
	writer    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c001")
	member    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c002")
	guest     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c003")
	requester = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c004")
	robot     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c005")
	agent     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c006")
	team      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c007")
)

// roleGate answers as roles would: the writer writes and reads, the member, the service account and
// the agent read the hub and the workspace, the guest reads the hub alone, the requester nothing.
type roleGate struct{ hubOnly []shared.ID }

func (g roleGate) allows(actor appshared.ActorContext, request access.Request) bool {
	id := actor.AccountID
	if request.Permission == service.PermissionWriteItems {
		return id == writer
	}
	switch id {
	case writer, member, robot, agent:
		return true
	case guest:
		return len(request.Path) > 1 || slices.Contains(g.hubOnly, guest)
	}
	return false
}

func (g roleGate) Authorize(_ context.Context, actor appshared.ActorContext, request access.Request) error {
	if !g.allows(actor, request) {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func (g roleGate) Permits(_ context.Context, actor appshared.ActorContext, request access.Request) (bool, error) {
	return g.allows(actor, request), nil
}

func (g roleGate) Permitted(_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i := range paths {
		out[i] = g.allows(actor, request)
	}
	return out, nil
}

// postStore keeps posts in memory.
type postStore struct {
	posts     map[shared.ID]domain.Post
	titles    map[shared.ID]string
	seen      map[shared.ID][]shared.ID
	confirmed map[shared.ID][]shared.ID
	replies   []repository.Reply
	reactions map[shared.ID]map[shared.ID]domain.Reaction
	groups    map[shared.ID][]shared.ID
	people    []shared.ID
}

func newPostStore() *postStore {
	return &postStore{
		posts: map[shared.ID]domain.Post{}, titles: map[shared.ID]string{}, seen: map[shared.ID][]shared.ID{},
		confirmed: map[shared.ID][]shared.ID{}, reactions: map[shared.ID]map[shared.ID]domain.Reaction{},
		groups: map[shared.ID][]shared.ID{member: {team}}, people: []shared.ID{writer, member, guest, requester},
	}
}

func (s *postStore) Insert(_ context.Context, post domain.Post) error {
	s.posts[post.PageID] = post
	return nil
}
func (s *postStore) Find(_ context.Context, id shared.ID) (domain.Post, error) {
	post, ok := s.posts[id]
	if !ok {
		return domain.Post{}, shared.ErrNotFound.WithDetail("posts.not_found")
	}
	return post, nil
}
func (s *postStore) FindForUpdate(ctx context.Context, id shared.ID) (domain.Post, error) {
	return s.Find(ctx, id)
}
func (s *postStore) Update(_ context.Context, post domain.Post) error {
	s.posts[post.PageID] = post
	return nil
}
func (s *postStore) PostPages(_ context.Context, ids []shared.ID) (map[shared.ID]bool, error) {
	out := map[shared.ID]bool{}
	for _, id := range ids {
		_, out[id] = s.posts[id]
	}
	return out, nil
}
func (s *postStore) List(_ context.Context, hubID, account shared.ID, f repository.PostFilter, _ time.Time, _ int) ([]repository.PostRow, error) {
	var out []repository.PostRow
	for id, post := range s.posts {
		seen := slices.Contains(s.seen[id], account)
		if (!hubID.IsZero() && post.HubID != hubID) || (f.Unread && seen) || (!f.PageID.IsZero() && f.PageID != id) ||
			(f.Published && post.PublishedAt == nil) {
			continue
		}
		out = append(out, repository.PostRow{
			Post: post, Title: s.titles[id], CreatedBy: writer, SeenCount: len(s.seen[id]), Seen: seen,
			Confirmed: slices.Contains(s.confirmed[id], account),
		})
	}
	return out, nil
}
func (s *postStore) MarkSeen(_ context.Context, id, account shared.ID, _ time.Time) error {
	if !slices.Contains(s.seen[id], account) {
		s.seen[id] = append(s.seen[id], account)
	}
	return nil
}
func (s *postStore) Confirm(_ context.Context, id, account shared.ID, _ time.Time) error {
	if !slices.Contains(s.confirmed[id], account) {
		s.confirmed[id] = append(s.confirmed[id], account)
	}
	return nil
}
func (s *postStore) ConfirmedBy(_ context.Context, id shared.ID) ([]shared.ID, error) {
	return slices.Clone(s.confirmed[id]), nil
}
func (s *postStore) PeopleReaching(context.Context, shared.ID, int) ([]shared.ID, error) {
	return slices.Clone(s.people), nil
}
func (s *postStore) GroupsOf(_ context.Context, accounts []shared.ID) (map[shared.ID][]shared.ID, error) {
	out := map[shared.ID][]shared.ID{}
	for _, a := range accounts {
		out[a] = s.groups[a]
	}
	return out, nil
}
func (s *postStore) InsertReply(_ context.Context, r repository.Reply) error {
	s.replies = append(s.replies, r)
	return nil
}
func (s *postStore) FindReply(_ context.Context, id shared.ID) (repository.Reply, error) {
	for _, r := range s.replies {
		if r.ID == id {
			return r, nil
		}
	}
	return repository.Reply{}, shared.ErrNotFound.WithDetail("posts.reply_not_found")
}
func (s *postStore) Replies(_ context.Context, id shared.ID, _ int) ([]repository.Reply, error) {
	return slices.DeleteFunc(slices.Clone(s.replies), func(r repository.Reply) bool { return r.PageID != id }), nil
}
func (s *postStore) TakeBackReply(_ context.Context, id shared.ID, at time.Time) error {
	for i := range s.replies {
		if s.replies[i].ID == id {
			s.replies[i].Body, s.replies[i].DeletedAt = "", &at
		}
	}
	return nil
}
func (s *postStore) Reactions(_ context.Context, _, account shared.ID) ([]repository.ReactionCount, error) {
	var out []repository.ReactionCount
	for reply, by := range s.reactions {
		for who, r := range by {
			out = append(out, repository.ReactionCount{ReplyID: reply, Reaction: r, Count: 1, Mine: who == account})
		}
	}
	return out, nil
}
func (s *postStore) React(_ context.Context, reply, account shared.ID, r domain.Reaction) error {
	if s.reactions[reply] == nil {
		s.reactions[reply] = map[shared.ID]domain.Reaction{}
	}
	s.reactions[reply][account] = r
	return nil
}
func (s *postStore) Unreact(_ context.Context, reply, account shared.ID, _ domain.Reaction) error {
	delete(s.reactions[reply], account)
	return nil
}

// timers records what was armed.
type timers struct{ armed map[shared.ID]time.Duration }

func (t *timers) Arm(_ context.Context, _ shared.ID, _ calendar.Purpose, subject shared.ID, _ string, _ calendar.BusinessCalendar, budget time.Duration) (calendar.Timer, error) {
	t.armed[subject] = budget
	return calendar.Timer{}, nil
}
func (t *timers) Stop(_ context.Context, _ calendar.Purpose, subject shared.ID, _ string) (calendar.Timer, error) {
	delete(t.armed, subject)
	return calendar.Timer{}, nil
}

// postTold records who a post was told to.
type postTold struct{ published, reminded [][]shared.ID }

func (t *postTold) PostPublished(_ context.Context, _ shared.ID, _ domain.Post, to []shared.ID) error {
	t.published = append(t.published, to)
	return nil
}
func (t *postTold) PostReminder(_ context.Context, _, _ shared.ID, _ domain.Post, to []shared.ID) error {
	t.reminded = append(t.reminded, to)
	return nil
}

type postFixture struct {
	fixture
	posts  *postStore
	timers *timers
	told   *postTold
}

func newPostFixture() postFixture {
	f := postFixture{fixture: newFixture(), posts: newPostStore(), timers: &timers{armed: map[shared.ID]time.Duration{}}, told: &postTold{}}
	f.space.Authorizer, f.space.Reader = roleGate{}, roleGate{}
	f.space.Posting = Posting{Posts: f.posts, Timers: f.timers, Notify: f.told}
	return f
}

// actorOf is each account kind as a door hands it to a use case.
func actorOf(account shared.ID) appshared.ActorContext {
	actor := as(account)
	switch account {
	case robot:
		actor.Kind = appshared.ActorServiceAccount
	case agent:
		actor.Kind, actor.OnBehalfOf = appshared.ActorAIAgent, member
	}
	return actor
}

// call runs a use case as every door does: through its catalogue entry.
func call(t *testing.T, d usecase.Descriptor, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return d.Handler.Invoke(context.Background(), actor, in)
}

func (f postFixture) post(t *testing.T, in usecase.Input) shared.ID {
	t.Helper()
	in["hub_id"], in["title"] = hub.String(), "Release 4.2"
	out, err := call(t, CreatePost{Space: f.space}.Descriptor(), actorOf(writer), in)
	if err != nil {
		t.Fatal(err)
	}
	id := shared.MustParseID(out["page_id"].(string))
	f.posts.titles[id] = "Release 4.2"
	return id
}

// The matrix: a post's audience and moment × account kind, each through the catalogue entry the
// REST, MCP and hubctl doors call alike (the parity gate holds them to one). Nobody without READ on
// the hub sees a post; a scheduled one only its writers; one narrowed to a group only the group's
// readers - and an agent no more than its person.
func TestAPostReachesItsAudienceAloneThroughEveryDoor(t *testing.T) {
	ahead := start.Add(time.Hour).Format(time.RFC3339)
	for _, row := range []struct {
		name     string
		in       usecase.Input
		sees     []shared.ID
		hiddenTo []shared.ID
	}{
		{"published to the hub", usecase.Input{"publish_at": start.Format(time.RFC3339)},
			[]shared.ID{writer, member, guest, robot, agent}, []shared.ID{requester}},
		{"scheduled", usecase.Input{"publish_at": ahead},
			[]shared.ID{writer}, []shared.ID{member, guest, robot, agent, requester}},
		{"a draft", usecase.Input{},
			[]shared.ID{writer}, []shared.ID{member, guest, robot, agent, requester}},
		{"narrowed to a group", usecase.Input{"publish_at": start.Format(time.RFC3339), "audience": "GROUPS", "group_ids": []any{team.String()}},
			[]shared.ID{writer, member, agent}, []shared.ID{guest, robot, requester}},
	} {
		t.Run(row.name, func(t *testing.T) {
			f := newPostFixture()
			id := f.post(t, row.in)
			f.store.pages[id] = domain.Page{ID: id, HubID: hub, Title: "Release 4.2", Version: 1}
			for _, account := range append(slices.Clone(row.sees), row.hiddenTo...) {
				wants := slices.Contains(row.sees, account)
				_, readErr := call(t, ReadPost{Space: f.space}.Descriptor(), actorOf(account), usecase.Input{"page_id": id.String()})
				list, listErr := call(t, ListHubPosts{Space: f.space}.Descriptor(), actorOf(account), usecase.Input{"hub_id": hub.String()})
				listed := listErr == nil && len(list["data"].([]any)) == 1
				_, pageErr := (ReadPage{Space: f.space}).Execute(context.Background(), actorOf(account), id)
				if (readErr == nil) != wants || listed != wants || (pageErr == nil) != wants {
					t.Errorf("account %s: read %v, listed %v, page %v; wants seen %v", account, readErr, listed, pageErr, wants)
				}
			}
		})
	}
}

// New for you (UC-DOC-18 check 5): the published posts a person has not seen, across hubs, each
// judged at its hub - never a scheduled one or a draft for a reader, never one outside the audience,
// and gone once seen.
func TestNewForYouListsTheUnreadPostsTheReaderIsFor(t *testing.T) {
	f := newPostFixture()
	published := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339)})
	narrowed := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339), "audience": "GROUPS", "group_ids": []any{team.String()}})
	f.post(t, usecase.Input{"publish_at": start.Add(time.Hour).Format(time.RFC3339)})
	f.post(t, usecase.Input{})
	unread := func(account shared.ID) []string {
		out, err := call(t, ListUnreadPosts{Space: f.space}.Descriptor(), actorOf(account), usecase.Input{})
		if err != nil {
			t.Fatalf("account %s: %v", account, err)
		}
		var ids []string
		for _, row := range out["data"].([]any) {
			ids = append(ids, row.(usecase.Output)["page_id"].(string))
		}
		slices.Sort(ids)
		return ids
	}
	both := []string{published.String(), narrowed.String()}
	slices.Sort(both)
	for _, row := range []struct {
		account shared.ID
		wants   []string
	}{
		{member, both}, {agent, both}, {guest, []string{published.String()}}, {requester, nil},
	} {
		if got := unread(row.account); !slices.Equal(got, row.wants) {
			t.Errorf("account %s: new for you %v, wants %v", row.account, got, row.wants)
		}
	}
	_ = f.posts.MarkSeen(context.Background(), published, member, start)
	if got := unread(member); !slices.Equal(got, []string{narrowed.String()}) {
		t.Errorf("after seeing one: %v", got)
	}
}

// The scheduler publishes a scheduled post at the moment asked, by the clock, and arms the next
// leg of one moved further ahead (B-3); the post it published is told to its audience as a
// writer's publishing is (UC-DOC-17 check 5).
func TestTheSchedulerPublishesAPostAtItsMoment(t *testing.T) {
	f := newPostFixture()
	at := start.Add(2 * time.Hour)
	id := f.post(t, usecase.Input{"publish_at": at.Format(time.RFC3339), "notify": true})
	if f.timers.armed[id] != 2*time.Hour {
		t.Fatalf("armed %v; wants the timer at the post's moment", f.timers.armed)
	}
	elapsed := PostPublishElapsed{Pages: f.store, Posts: f.posts, Timers: f.timers, Clock: f.clock,
		Published: f.space.ScheduledPostPublished}
	f.store.pages[id] = domain.Page{ID: id, HubID: hub}
	f.clock.at = at.Add(time.Minute)
	if err := elapsed.Elapsed(context.Background(), appshared.ActorContext{}, calendar.Timer{TenantID: tenant, SubjectID: id}); err != nil {
		t.Fatal(err)
	}
	if len(f.told.published) != 1 || slices.Contains(f.told.published[0], requester) || len(f.told.published[0]) == 0 {
		t.Errorf("told %v; wants the hub's readers once, never the requester", f.told.published)
	}
	if post := f.posts.posts[id]; post.PublishedAt == nil || !post.PublishedAt.Equal(at) {
		t.Errorf("published %+v; wants at the moment asked", post.PublishedAt)
	}
	// Publishing it by hand meanwhile leaves it as it is.
	before := *f.posts.posts[id].PublishedAt
	f.clock.at = at.Add(time.Hour)
	_ = elapsed.Elapsed(context.Background(), appshared.ActorContext{}, calendar.Timer{TenantID: tenant, SubjectID: id})
	if !f.posts.posts[id].PublishedAt.Equal(before) {
		t.Error("a published post was published again")
	}
}

// Seen by N carries no names; a read confirmation's standing and who confirmed go to the writers
// alone; only a person confirms; a reminder reaches those of the audience who have not confirmed,
// and the publishing tells the audience - never anybody outside the hub's readers (B-5, B-6).
func TestReadConfirmationsAreTheWritersAlone(t *testing.T) {
	f := newPostFixture()
	by := start.Add(48 * time.Hour).Format(time.RFC3339)
	id := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339), "confirm_by": by})
	if len(f.told.published) != 1 || !slices.Equal(f.told.published[0], []shared.ID{member, guest}) {
		t.Errorf("told %v; wants the hub's readers but the writer, never the requester", f.told.published)
	}
	page := usecase.Input{"page_id": id.String()}
	if _, err := call(t, ConfirmPost{Space: f.space}.Descriptor(), actorOf(member), page); err != nil {
		t.Fatal(err)
	}
	for _, account := range []shared.ID{agent, robot} {
		if _, err := call(t, ConfirmPost{Space: f.space}.Descriptor(), actorOf(account), page); code(err) != "posts.person_confirms" {
			t.Errorf("%s confirmed: %v", account, err)
		}
	}
	out, err := call(t, ReadPost{Space: f.space}.Descriptor(), actorOf(member), page)
	if err != nil || out["seen_count"] != 1 || out["confirmed"] != true || out["confirmations"] != nil {
		t.Errorf("a member read %v, %v; wants the count, their own confirmation, no standing", out, err)
	}
	if _, err := call(t, ReadPostConfirmations{Space: f.space}.Descriptor(), actorOf(member), page); err == nil {
		t.Error("a reader saw who confirmed")
	}
	standing, err := call(t, ReadPostConfirmations{Space: f.space}.Descriptor(), actorOf(writer), page)
	if err != nil || standing["confirmed"] != 1 || standing["audience"] != 3 || standing["may_remind"] != true ||
		!slices.Equal(standing["confirmed_ids"].([]string), []string{member.String()}) {
		t.Errorf("a writer read %v, %v; wants 1 of 3 with the name", standing, err)
	}
	reminded, err := call(t, RemindPost{Space: f.space}.Descriptor(), actorOf(writer), page)
	if err != nil || reminded["reminded"] != 2 || !slices.Equal(f.told.reminded[0], []shared.ID{writer, guest}) {
		t.Errorf("reminded %v %v, %v", reminded, f.told.reminded, err)
	}
	for _, entry := range f.trail.entries {
		if entry.Action == PostConfirmedAction || entry.Action == PostSeenAction {
			t.Errorf("the trail recorded who read the post: %+v", entry)
		}
	}
}

// "Everyone in the workspace" is offered only where every member of the workspace reads the hub,
// and there it is the hub's readers; a hub a member cannot read never offers it (B-6).
func TestEveryoneInTheWorkspaceIsTheHubsReadersWhereAllReadIt(t *testing.T) {
	f := newPostFixture()
	f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339)})
	list, err := call(t, ListHubPosts{Space: f.space}.Descriptor(), actorOf(writer), usecase.Input{"hub_id": hub.String()})
	if err != nil || list["workspace_wide"] != true || list["data"].([]any)[0].(usecase.Output)["everyone_in_workspace"] != true {
		t.Errorf("list %v, %v; wants everyone in the workspace", list, err)
	}
	f.space.Authorizer = closedGate{}
	list, err = call(t, ListHubPosts{Space: f.space}.Descriptor(), actorOf(writer), usecase.Input{"hub_id": hub.String()})
	if err != nil || list["workspace_wide"] != false {
		t.Errorf("list %v, %v; wants no workspace where a member cannot read the hub", list, err)
	}
}

// closedGate is roleGate with the member unable to read the hub.
type closedGate struct{ roleGate }

func (g closedGate) Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	if ok, _ := g.Permits(ctx, actor, request); !ok {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func (g closedGate) Permits(_ context.Context, actor appshared.ActorContext, request access.Request) (bool, error) {
	if actor.AccountID == member && len(request.Path) > 1 {
		return false, nil
	}
	return g.allows(actor, request), nil
}

// The discussion: anybody the post is for replies and reacts, one level deep, a person only reacts;
// a reply is taken back by its author or a writer (B-4).
func TestAPostIsDiscussedWithReplyAndReaction(t *testing.T) {
	f := newPostFixture()
	id := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339)})
	page := usecase.Input{"page_id": id.String(), "body": "Great news"}
	reply, err := call(t, AddPostReply{Space: f.space}.Descriptor(), actorOf(member), page)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, AddPostReply{Space: f.space}.Descriptor(), actorOf(requester), page); err == nil {
		t.Error("a requester replied")
	}
	answer := usecase.Input{"page_id": id.String(), "body": "Indeed", "parent_id": reply["id"]}
	second, err := call(t, AddPostReply{Space: f.space}.Descriptor(), actorOf(guest), answer)
	if err != nil {
		t.Fatal(err)
	}
	answer["parent_id"] = second["id"]
	if _, err := call(t, AddPostReply{Space: f.space}.Descriptor(), actorOf(guest), answer); code(err) != "posts.reply_parent" {
		t.Errorf("a second level: %v", err)
	}
	react := usecase.Input{"page_id": id.String(), "reply_id": reply["id"], "reaction": "THANKS"}
	reacted, err := call(t, ReactToPostReply{Space: f.space}.Descriptor(), actorOf(guest), react)
	if err != nil || len(reacted["reactions"].([]any)) != 1 {
		t.Errorf("reacted %v, %v", reacted, err)
	}
	if _, err := call(t, ReactToPostReply{Space: f.space}.Descriptor(), actorOf(agent), react); code(err) != "posts.person_reacts" {
		t.Errorf("an agent reacted: %v", err)
	}
	gone := usecase.Input{"page_id": id.String(), "reply_id": reply["id"]}
	if _, err := call(t, DeletePostReply{Space: f.space}.Descriptor(), actorOf(guest), gone); code(err) != "posts.reply_not_yours" {
		t.Errorf("another's reply taken back: %v", err)
	}
	if _, err := call(t, DeletePostReply{Space: f.space}.Descriptor(), actorOf(writer), gone); err != nil {
		t.Errorf("a writer took a reply back: %v", err)
	}
}

// A post has its own publishing: in a hub whose pages wait in drafts (UC-DOC-15) its words are
// still written to its page, never into a draft, and its live document is open to its writers.
func TestAPostsWordsNeverWaitInAPageDraft(t *testing.T) {
	f := newPostFixture()
	ctx := context.Background()
	f.space.Drafting = Drafting{
		Drafts:   &draftStore{drafts: map[shared.ID]domain.Draft{}},
		Settings: &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}},
	}
	if _, err := (SetSpaceDrafts{Space: f.space}).Execute(ctx, as(writer), SetSpaceDraftsCommand{HubID: hub, Drafts: true}); err != nil {
		t.Fatal(err)
	}
	id := f.post(t, usecase.Input{"publish_at": start.Format(time.RFC3339)})
	page := f.store.pages[id]
	written, err := (UpdatePage{Space: f.space}).write(ctx, actorOf(writer), UpdatePageCommand{
		PageID: id, Change: domain.Change{Body: ptr("Release 4.2 is out")}, ExpectedVersion: page.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if written.Draft != nil || written.Drafts || written.Page.Body != "Release 4.2 is out" {
		t.Fatalf("the post's words went to a draft: %+v", written)
	}
	read, err := (ReadPage{Space: f.space}).Execute(ctx, actorOf(writer), id)
	if err != nil || read.Drafts || read.Draft != nil || read.Page.Body != "Release 4.2 is out" {
		t.Fatalf("the post's page reads %+v, %v", read, err)
	}
}
