// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The posts' use case names and trail codes (work-surface concept E-15). Who has seen or confirmed
// a post, and who reacted, is personal data and never reaches the trail (rule 10).
const (
	ListHubPostsName          = "ListHubPosts"
	CreatePostName            = "CreatePost"
	ReadPostName              = "ReadPost"
	SetPostSettingsName       = "SetPostSettings"
	PublishPostName           = "PublishPost"
	MarkPostSeenName          = "MarkPostSeen"
	ConfirmPostName           = "ConfirmPost"
	ReadPostConfirmationsName = "ReadPostConfirmations"
	RemindPostName            = "RemindPost"
	ListPostRepliesName       = "ListPostReplies"
	AddPostReplyName          = "AddPostReply"
	DeletePostReplyName       = "DeletePostReply"
	ReactToPostReplyName      = "ReactToPostReply"
	UnreactToPostReplyName    = "UnreactToPostReply"

	PostReadAction      audit.Action = "post.read"
	PostCreatedAction   audit.Action = "post.created"
	PostChangedAction   audit.Action = "post.changed"
	PostPublishedAction audit.Action = "post.published"
	PostSeenAction      audit.Action = "post.seen"
	PostConfirmedAction audit.Action = "post.confirmed"
	PostRemindedAction  audit.Action = "post.reminded"
	PostRepliedAction   audit.Action = "post_reply.added"
	PostReplyGoneAction audit.Action = "post_reply.deleted"
	PostReactedAction   audit.Action = "post_reply.reacted"

	postTarget  = "post"
	replyTarget = "post_reply"
	postTimer   = "publish"
	// maxPosts bounds a hub's list; maxPeople the people an audience is judged among.
	maxPosts   = 500
	maxReplies = 2000
	maxPeople  = 5000
)

// Posting is what posts need beside the space; a nil Posts offers none.
type Posting struct {
	Posts repository.Posts
	// Timers publishes a scheduled post at its moment; nil publishes it only when a writer does.
	Timers ReviewTimers
	// Notify tells a post's audience it was published, and those who have not confirmed it that a
	// writer reminds them; nil tells nobody.
	Notify PostNotifier
}

// PostNotifier tells people about a post. It is handed the people the application judged (rule 2).
type PostNotifier interface {
	PostPublished(ctx context.Context, tenant shared.ID, post domain.Post, recipients []shared.ID) error
	PostReminder(ctx context.Context, tenant shared.ID, post domain.Post, recipients []shared.ID) error
}

func (s Space) posts() (repository.Posts, error) {
	if s.Posting.Posts == nil {
		return nil, shared.ErrUnavailable.WithDetail("posts.unavailable")
	}
	return s.Posting.Posts, nil
}

// person is the account whose groups and confirmations count for the actor: an agent's person.
func person(actor appshared.ActorContext) shared.ID {
	if !actor.OnBehalfOf.IsZero() {
		return actor.OnBehalfOf
	}
	return actor.AccountID
}

// isPerson answers whether the actor is a person acting as themselves: only they see, confirm and
// count among an audience - never a service account, an automation or an agent.
func isPerson(actor appshared.ActorContext) bool {
	return actor.Kind == appshared.ActorUser && actor.OnBehalfOf.IsZero()
}

// postReader judges what decides whether the actor sees a hub's posts (B-6): READ and WRITE_ITEMS
// at the hub and the person's groups. Asked without a record.
func (s Space) postReader(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (domain.Reader, error) {
	reads, err := s.Authorizer.Permits(ctx, actor, s.request(hubID, service.PermissionRead, PostReadAction, pagesRead, postTarget, hubID))
	if err != nil || !reads {
		return domain.Reader{}, err
	}
	writes, err := s.mayWrite(ctx, actor, hubID)
	if err != nil {
		return domain.Reader{}, err
	}
	var groups map[shared.ID][]shared.ID
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		groups, err = s.Posting.Posts.GroupsOf(ctx, []shared.ID{person(actor)})
		return err
	})
	return domain.Reader{ReadsHub: true, WritesHub: writes, Groups: groups[person(actor)]}, err
}

// hidePosts refuses a post's page to a reader the post is not for, as if it were not there: every
// read of a page passes here, so a scheduled post's words reach its writers alone (B-3).
func (s Space) hidePosts(ctx context.Context, actor appshared.ActorContext, page domain.Page) error {
	if s.Posting.Posts == nil {
		return nil
	}
	var post domain.Post
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		post, err = s.Posting.Posts.Find(ctx, page.ID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	reader, err := s.postReader(ctx, actor, page.HubID)
	if err != nil {
		return err
	}
	if !post.Sees(reader, s.Clock.Now()) {
		return shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return nil
}

// readersOf keeps the people who read the hub, each judged by the authoriser as themselves.
func (s Space) readersOf(ctx context.Context, tenant shared.ID, path []identity.Scope, people []shared.ID) ([]shared.ID, error) {
	kept := make([]shared.ID, 0, len(people))
	for _, id := range people {
		reads, err := s.Authorizer.Permits(ctx, appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: tenant, AccountID: id, Scopes: []string{pagesRead},
		}, access.Request{Permission: service.PermissionRead, Path: path, Action: PostReadAction, TargetType: postTarget})
		if err != nil {
			return nil, err
		}
		if reads {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

// audience is the people a published post is for (B-6): the hub's readers, narrowed to the groups
// it names. Nobody outside the hub's readers is ever in it.
func (s Space) audience(ctx context.Context, actor appshared.ActorContext, post domain.Post) ([]shared.ID, error) {
	var people []shared.ID
	var groups map[shared.ID][]shared.ID
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if people, err = s.Posting.Posts.PeopleReaching(ctx, post.HubID, maxPeople); err != nil {
			return err
		}
		if post.Audience == domain.AudienceGroups {
			groups, err = s.Posting.Posts.GroupsOf(ctx, people)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if post.Audience == domain.AudienceGroups {
		people = slices.DeleteFunc(people, func(id shared.ID) bool {
			return !slices.ContainsFunc(groups[id], func(g shared.ID) bool { return slices.Contains(post.GroupIDs, g) })
		})
	}
	return s.readersOf(ctx, actor.TenantID, hubPath(post.HubID), people)
}

// workspaceWide answers whether every member of the workspace reads the hub: only then is "everyone
// in the workspace" offered, and it is the hub's readers by another name.
func (s Space) workspaceWide(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (bool, error) {
	var people []shared.ID
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		people, err = s.Posting.Posts.PeopleReaching(ctx, "", maxPeople)
		return err
	})
	if err != nil {
		return false, err
	}
	members, err := s.readersOf(ctx, actor.TenantID, []identity.Scope{identity.TenantScope()}, people)
	if err != nil || len(members) == 0 {
		return false, err
	}
	readers, err := s.readersOf(ctx, actor.TenantID, hubPath(hubID), members)
	return err == nil && len(readers) == len(members), err
}

// confirmations is how a post's read confirmation stands, for its writers; nil where it asks none.
func (s Space) confirmations(ctx context.Context, actor appshared.ActorContext, post domain.Post) (*domain.Confirmations, []shared.ID, error) {
	if post.ConfirmBy == nil {
		return nil, nil, nil
	}
	audience, err := s.audience(ctx, actor, post)
	if err != nil {
		return nil, nil, err
	}
	var confirmed []shared.ID
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		confirmed, err = s.Posting.Posts.ConfirmedBy(ctx, post.PageID)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	// Somebody who confirmed and has since left the audience is not counted against it.
	confirmed = slices.DeleteFunc(confirmed, func(id shared.ID) bool { return !slices.Contains(audience, id) })
	return &domain.Confirmations{Confirmed: len(confirmed), Audience: len(audience), ConfirmBy: *post.ConfirmBy}, confirmed, nil
}

func confirmationsOutput(c *domain.Confirmations, who []shared.ID, now time.Time) any {
	if c == nil {
		return nil
	}
	out := usecase.Output{
		"confirmed": c.Confirmed, "audience": c.Audience, "confirm_by": c.ConfirmBy,
		"may_remind": c.MayRemind(), "overdue": c.IsOverdue(now),
	}
	if who != nil {
		out["confirmed_ids"] = idStrings(who)
	}
	return out
}

// PostOutput is the shape every door returns (api/openapi.yaml, schema Post).
func PostOutput(row repository.PostRow, writes, workspaceWide bool, confirmations *domain.Confirmations, now time.Time) usecase.Output {
	p := row.Post
	groups := idStrings(p.GroupIDs)
	if groups == nil {
		groups = []string{}
	}
	return usecase.Output{
		"page_id": p.PageID.String(), "hub_id": p.HubID.String(), "title": row.Title, "icon": nilIfEmpty(row.Icon),
		"template": nilIfEmpty(string(p.Template)), "state": string(p.State(now)),
		"publish_at": timeOrNil(p.PublishAt), "published_at": timeOrNil(p.PublishedAt),
		"featured_until": timeOrNil(p.FeaturedUntil), "featured": p.IsFeatured(now),
		"audience": string(p.Audience), "group_ids": groups,
		"everyone_in_workspace": writes && workspaceWide && p.Audience == domain.AudienceHub,
		"discussion":            p.Discussion, "notify": p.Notify, "confirm_by": timeOrNil(p.ConfirmBy),
		"created_by": idOrNil(row.CreatedBy), "created_at": row.CreatedAt, "seen_count": row.SeenCount,
		"seen": row.Seen, "confirmed": row.Confirmed, "may_write": writes,
		"confirmations": confirmationsOutput(confirmations, nil, now),
	}
}

// visiblePost reads a post and its row for the actor, not found for anybody it is not for.
func (s Space) visiblePost(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action) (repository.PostRow, domain.Reader, error) {
	posts, err := s.posts()
	if err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	if err := actor.RequireScope(pagesRead); err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	var post domain.Post
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		post, err = posts.Find(ctx, pageID)
		return err
	})
	if err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	if err := s.Authorizer.Authorize(ctx, actor, s.request(post.HubID, service.PermissionRead, action, pagesRead, postTarget, pageID)); err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	reader, err := s.postReader(ctx, actor, post.HubID)
	if err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	now := s.Clock.Now()
	if !post.Sees(reader, now) {
		return repository.PostRow{}, domain.Reader{}, shared.ErrNotFound.WithDetail("posts.not_found")
	}
	var rows []repository.PostRow
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = posts.List(ctx, post.HubID, person(actor), repository.PostFilter{PageID: pageID}, now, 1)
		return err
	})
	if err != nil {
		return repository.PostRow{}, domain.Reader{}, err
	}
	if len(rows) == 0 {
		return repository.PostRow{}, domain.Reader{}, shared.ErrNotFound.WithDetail("posts.not_found")
	}
	return rows[0], reader, nil
}

// answer is a post as the actor sees it: a writer also gets how its confirmation stands.
func (s Space) answer(ctx context.Context, actor appshared.ActorContext, row repository.PostRow, reader domain.Reader) (usecase.Output, error) {
	now := s.Clock.Now()
	var standing *domain.Confirmations
	wide := false
	if reader.WritesHub {
		var err error
		if standing, _, err = s.confirmations(ctx, actor, row.Post); err != nil {
			return nil, err
		}
		if wide, err = s.workspaceWide(ctx, actor, row.Post.HubID); err != nil {
			return nil, err
		}
	}
	return PostOutput(row, reader.WritesHub, wide, standing, now), nil
}

// settingsOf reads what a writer asks of a post.
func settingsOf(in usecase.Input, now time.Time) (domain.PostSettings, error) {
	var settings domain.PostSettings
	var err error
	for field, into := range map[string]**time.Time{
		"publish_at": &settings.PublishAt, "featured_until": &settings.FeaturedUntil, "confirm_by": &settings.ConfirmBy,
	} {
		if *into, err = optionalMoment(in, field); err != nil {
			return domain.PostSettings{}, err
		}
	}
	if settings.GroupIDs, err = in.IDList("group_ids"); err != nil {
		return domain.PostSettings{}, err
	}
	settings.Template = domain.PostTemplate(in.String("template"))
	settings.Audience = domain.Audience(in.String("audience"))
	settings.Discussion = !in.Present("discussion") || in.Bool("discussion")
	settings.Notify = !in.Present("notify") || in.Bool("notify")
	return domain.NewPostSettings(settings, now)
}

// applied is the post with a writer's settings; a published post keeps its moment.
func applied(post domain.Post, settings domain.PostSettings) domain.Post {
	post.Template, post.FeaturedUntil, post.Audience = settings.Template, settings.FeaturedUntil, settings.Audience
	post.GroupIDs, post.Discussion, post.Notify, post.ConfirmBy = settings.GroupIDs, settings.Discussion, settings.Notify, settings.ConfirmBy
	if post.PublishedAt == nil {
		post.PublishAt = settings.PublishAt
	}
	return post
}

// settled publishes a post whose moment has come and arms the timer of one scheduled ahead,
// inside the caller's transaction; it answers whether the post went out now.
func (s Space) settled(ctx context.Context, tenant shared.ID, post domain.Post, now time.Time) (domain.Post, bool, error) {
	timers := s.Posting.Timers
	if timers != nil {
		if _, err := timers.Stop(ctx, calendar.PurposePostPublish, post.PageID, postTimer); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return post, false, err
		}
	}
	if post.IsDue(now) {
		return post.Published(now), true, nil
	}
	if timers != nil && post.State(now) == domain.PostScheduled {
		budget := min(post.PublishAt.Sub(now), calendar.MaxBudget)
		if _, err := timers.Arm(ctx, tenant, calendar.PurposePostPublish, post.PageID, postTimer, calendar.Continuous(), max(budget, time.Second)); err != nil {
			return post, false, err
		}
	}
	return post, false, nil
}

// told tells a just published post's audience, when it asks for that.
func (s Space) told(ctx context.Context, actor appshared.ActorContext, post domain.Post) error {
	if s.Posting.Notify == nil || !post.Notify {
		return nil
	}
	audience, err := s.audience(ctx, actor, post)
	if err != nil {
		return err
	}
	audience = slices.DeleteFunc(audience, func(id shared.ID) bool { return id == actor.AccountID })
	return s.Posting.Notify.PostPublished(ctx, actor.TenantID, post, audience)
}

// writablePost reads a post for a writer of its hub, recording a refusal.
func (s Space) writablePost(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action) (domain.Post, error) {
	posts, err := s.posts()
	if err != nil {
		return domain.Post{}, err
	}
	var post domain.Post
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		post, err = posts.Find(ctx, pageID)
		return err
	})
	if err != nil {
		return domain.Post{}, err
	}
	return post, s.authorizeWrite(ctx, actor, post.HubID, pageID, action, postTarget)
}

// ListHubPosts reads a hub's posts the reader sees.
type ListHubPosts struct{ Space Space }

// Execute lists.
func (h ListHubPosts) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, filter string) (usecase.Output, error) {
	posts, err := h.Space.posts()
	if err != nil {
		return nil, err
	}
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(hubID, service.PermissionRead, PostReadAction, pagesRead, postTarget, hubID)); err != nil {
		return nil, err
	}
	reader, err := h.Space.postReader(ctx, actor, hubID)
	if err != nil {
		return nil, err
	}
	now := h.Space.Clock.Now()
	var rows []repository.PostRow
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, hubID); err != nil {
			return err
		}
		rows, err = posts.List(ctx, hubID, person(actor), repository.PostFilter{Mine: filter == "MINE", Unread: filter == "UNREAD"}, now, maxPosts)
		return err
	})
	if err != nil {
		return nil, err
	}
	wide := false
	if reader.WritesHub {
		if wide, err = h.Space.workspaceWide(ctx, actor, hubID); err != nil {
			return nil, err
		}
	}
	data := []any{}
	for _, row := range rows {
		if !row.Post.Sees(reader, now) || (filter == "SCHEDULED" && row.Post.State(now) != domain.PostScheduled) {
			continue
		}
		// The confirmations' standing is read one post at a time, for a writer, on the post itself.
		data = append(data, PostOutput(row, reader.WritesHub, wide, nil, now))
	}
	return usecase.Output{"data": data, "may_write": reader.WritesHub, "workspace_wide": reader.WritesHub && wide}, nil
}

// Descriptor is the catalogue entry.
func (h ListHubPosts) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListHubPostsName, Permission: service.PermissionRead,
		Summary: "Reads a hub's posts the caller sees, featured ones first, then newest first: published " +
			"ones for their audience, drafts and scheduled ones for the hub's writers only.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "filter", Kind: usecase.KindString, Enum: []string{"ALL", "UNREAD", "MINE", "SCHEDULED"}, Description: "Narrows the posts."},
		},
		Audit: usecase.AuditDeclaration{Action: PostReadAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			return h.Execute(ctx, actor, hubID, in.String("filter"))
		}),
	})
}

// postInput is the settings every writing use case declares.
func postInput(fields ...usecase.Field) []usecase.Field {
	return append(fields,
		usecase.Field{Name: "template", Kind: usecase.KindString, Enum: postTemplates(), Description: "The post template it started from."},
		usecase.Field{Name: "publish_at", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "When it goes out: now or earlier publishes, ahead schedules, none keeps a draft."},
		usecase.Field{Name: "featured_until", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "Keeps it on top until then."},
		usecase.Field{Name: "audience", Kind: usecase.KindString, Enum: []string{string(domain.AudienceHub), string(domain.AudienceGroups)}, Description: "The hub's readers, or those in the groups named."},
		usecase.Field{Name: "group_ids", Kind: usecase.KindIDList, Description: "The groups the audience narrows to."},
		usecase.Field{Name: "discussion", Kind: usecase.KindBool, Description: "Whether the discussion is open."},
		usecase.Field{Name: "notify", Kind: usecase.KindBool, Description: "Whether the hub's followers are told."},
		usecase.Field{Name: "confirm_by", Kind: usecase.KindString, Format: usecase.FormatDateTime, Description: "Asks its audience to confirm reading it by then."},
	)
}

func postTemplates() []string {
	out := make([]string, 0, len(domain.PostTemplates))
	for _, t := range domain.PostTemplates {
		out = append(out, string(t))
	}
	return out
}

// CreatePost writes a post: its page and its date, audience and confirmation.
type CreatePost struct{ Space Space }

// Execute writes.
func (h CreatePost) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, in usecase.Input) (usecase.Output, error) {
	posts, err := h.Space.posts()
	if err != nil {
		return nil, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, hubID, hubID, PostCreatedAction, postTarget); err != nil {
		return nil, err
	}
	now := h.Space.Clock.Now()
	settings, err := settingsOf(in, now)
	if err != nil {
		return nil, err
	}
	body := ""
	if raw := rawText(in, "body"); raw != nil {
		body = *raw
	}
	var post domain.Post
	var published bool
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, hubID); err != nil {
			return err
		}
		count, err := h.Space.Pages.Count(ctx, hubID)
		if err != nil {
			return err
		}
		if count >= domain.MaxPagesPerHub {
			return shared.ErrConflict.WithDetail("pages.space_full").WithParams(map[string]string{"maximum": "5000"})
		}
		last, err := h.Space.Pages.LastOrderKey(ctx, hubID, "")
		if err != nil {
			return err
		}
		key, err := service.OrderKeyAfter(last)
		if err != nil {
			return err
		}
		page, err = domain.NewPage(domain.NewPageInput{
			ID: h.Space.IDs.NewID(), TenantID: actor.TenantID, HubID: hubID, Title: in.String("title"), Body: body,
			OrderKey: key, CreatedBy: actor.AccountID, Now: now, Text: h.Space.Text,
		})
		if err != nil {
			return err
		}
		post = applied(domain.Post{PageID: page.ID, TenantID: actor.TenantID, HubID: hubID}, settings)
		if post, published, err = h.Space.settled(ctx, actor.TenantID, post, now); err != nil {
			return err
		}
		if err := posts.Insert(ctx, post); err != nil {
			return err
		}
		if page, err = h.Space.write(ctx, actor, page, false, now); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PostCreatedAction, postTarget, page.ID, postChanges(post), now)
	})
	if err != nil {
		return nil, err
	}
	if published {
		if err := h.Space.told(ctx, actor, post); err != nil {
			return nil, err
		}
	}
	row := repository.PostRow{Post: post, Title: page.Title, Icon: page.Icon, CreatedBy: actor.AccountID, CreatedAt: now}
	return h.Space.answer(ctx, actor, row, domain.Reader{ReadsHub: true, WritesHub: true})
}

// postChanges names a post's settings in the trail; nothing in them is a person's words.
func postChanges(post domain.Post) []audit.Change {
	return []audit.Change{
		{Field: "publish_at", Classification: audit.Open, To: timeOrNil(post.PublishAt)},
		{Field: "audience", Classification: audit.Open, To: string(post.Audience)},
		{Field: "featured_until", Classification: audit.Open, To: timeOrNil(post.FeaturedUntil)},
		{Field: "confirm_by", Classification: audit.Open, To: timeOrNil(post.ConfirmBy)},
	}
}

// Descriptor is the catalogue entry.
func (h CreatePost) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePostName, Permission: service.PermissionWriteItems,
		Summary: "Writes a post in a hub: a page with a date, published now, scheduled or kept as a " +
			"draft, for the hub's readers or those of them in the groups named.",
		SideEffects: "Writes the post's page and its settings, arms the timer of a scheduled post, tells " +
			"the audience of one published now, and writes an audit entry.",
		TokenScope: pagesWrite,
		Input: postInput(
			usecase.Field{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			usecase.Field{Name: "title", Kind: usecase.KindString, Required: true, Description: "The post's title."},
			usecase.Field{Name: "body", Kind: usecase.KindString, Description: "Markdown."},
		),
		Audit: usecase.AuditDeclaration{Action: PostCreatedAction, TargetType: postTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			return h.Execute(ctx, actor, hubID, in)
		}),
	})
}

// ReadPost reads a post for anybody it is for.
type ReadPost struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReadPost) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPostName, Permission: service.PermissionRead,
		Summary: "Reads a post's date, audience and state, how many have seen it and whether the " +
			"caller has seen and confirmed it; a writer also gets the confirmation's standing.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit: usecase.AuditDeclaration{Action: PostReadAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			row, reader, err := h.Space.visiblePost(ctx, actor, id, PostReadAction)
			if err != nil {
				return nil, err
			}
			return h.Space.answer(ctx, actor, row, reader)
		}),
	})
}

// change rewrites a post for a writer and answers it as it now stands.
func (s Space) change(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action,
	edit func(domain.Post, time.Time) (domain.Post, error)) (usecase.Output, error) {
	post, err := s.writablePost(ctx, actor, pageID, action)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	var published bool
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := s.Posting.Posts.FindForUpdate(ctx, pageID)
		if err != nil {
			return err
		}
		was := current.PublishedAt != nil
		if post, err = edit(current, now); err != nil {
			return err
		}
		if post, published, err = s.settled(ctx, actor.TenantID, post, now); err != nil {
			return err
		}
		published = published && !was
		if err := s.Posting.Posts.Update(ctx, post); err != nil {
			return err
		}
		return s.record(ctx, actor, action, postTarget, pageID, postChanges(post), now)
	})
	if err != nil {
		return nil, err
	}
	if published {
		if err := s.told(ctx, actor, post); err != nil {
			return nil, err
		}
	}
	row, reader, err := s.visiblePost(ctx, actor, pageID, PostReadAction)
	if err != nil {
		return nil, err
	}
	return s.answer(ctx, actor, row, reader)
}

// SetPostSettings schedules, features or narrows a post.
type SetPostSettings struct{ Space Space }

// Descriptor is the catalogue entry.
func (h SetPostSettings) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPostSettingsName, Permission: service.PermissionWriteItems,
		Summary: "Sets when a post goes out, until when it stays on top, for whom, whether it is " +
			"discussed and told, and until when its readers confirm it. A published post keeps its moment.",
		SideEffects: "Writes the settings, arms or stops the publishing timer, and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input:       postInput(usecase.Field{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}),
		Audit:       usecase.AuditDeclaration{Action: PostChangedAction, TargetType: postTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return h.Space.change(ctx, actor, id, PostChangedAction, func(post domain.Post, now time.Time) (domain.Post, error) {
				settings, err := settingsOf(in, now)
				if err != nil {
					return post, err
				}
				return applied(post, settings), nil
			})
		}),
	})
}

// PublishPost publishes a post now.
type PublishPost struct{ Space Space }

// Descriptor is the catalogue entry.
func (h PublishPost) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: PublishPostName, Permission: service.PermissionWriteItems,
		Summary:     "Publishes a draft or a scheduled post at once for its audience.",
		SideEffects: "Publishes the post, stops its timer, tells its audience, and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit:       usecase.AuditDeclaration{Action: PostPublishedAction, TargetType: postTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return h.Space.change(ctx, actor, id, PostPublishedAction, func(post domain.Post, now time.Time) (domain.Post, error) {
				if post.PublishedAt == nil {
					post.PublishAt = &now
				}
				return post, nil
			})
		}),
	})
}

// mark records a person's own act on a published post they see: seen or confirmed.
func (s Space) mark(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action, confirm bool) (usecase.Output, error) {
	row, reader, err := s.visiblePost(ctx, actor, pageID, action)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	if row.Post.State(now) != domain.PostPublished {
		return nil, shared.ErrConflict.WithDetail("posts.not_published")
	}
	if confirm && row.Post.ConfirmBy == nil {
		return nil, shared.ErrConflict.WithDetail("posts.confirmation_not_asked")
	}
	if !isPerson(actor) {
		// Only a person acting as themselves is seen or confirms: an agent's reading is not its
		// person's, and a service account is in no audience.
		if confirm {
			return nil, shared.ErrForbidden.WithDetail("posts.person_confirms")
		}
		return s.answer(ctx, actor, row, reader)
	}
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := s.Posting.Posts.MarkSeen(ctx, pageID, actor.AccountID, now); err != nil {
			return err
		}
		if confirm {
			return s.Posting.Posts.Confirm(ctx, pageID, actor.AccountID, now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if row, reader, err = s.visiblePost(ctx, actor, pageID, PostReadAction); err != nil {
		return nil, err
	}
	return s.answer(ctx, actor, row, reader)
}

// MarkPostSeen counts the reader among those who have seen a post.
type MarkPostSeen struct{ Space Space }

// Descriptor is the catalogue entry.
func (h MarkPostSeen) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: MarkPostSeenName, Permission: service.PermissionRead,
		Summary: "Counts the caller among those who have seen a published post; the count is shown to " +
			"everybody, the names to nobody. An agent's or a service account's reading is not counted.",
		SideEffects: "Records the person as having seen the post, once; no audit entry, since who read what is personal.",
		TokenScope:  pagesRead,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit:       usecase.AuditDeclaration{Action: PostSeenAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return h.Space.mark(ctx, actor, id, PostSeenAction, false)
		}),
	})
}

// ConfirmPost records that the reader confirms having read a post that asks for it.
type ConfirmPost struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ConfirmPost) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ConfirmPostName, Permission: service.PermissionRead,
		Summary: "Confirms that the caller read a published post that asks its audience for it. Only " +
			"a person confirms, never an agent for them.",
		SideEffects: "Records the confirmation, shown to the post's writers only; no audit entry, since it is personal.",
		TokenScope:  pagesRead,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit:       usecase.AuditDeclaration{Action: PostConfirmedAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return h.Space.mark(ctx, actor, id, PostConfirmedAction, true)
		}),
	})
}

// ReadPostConfirmations shows a post's writers who confirmed it.
type ReadPostConfirmations struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReadPostConfirmations) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPostConfirmationsName, Permission: service.PermissionWriteItems,
		Summary:     "Shows a post's writers how many of its audience confirmed it by when, and who did.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesWrite, ReadOnly: true,
		Input: []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit: usecase.AuditDeclaration{Action: PostReadAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			post, err := h.Space.writablePost(ctx, actor, id, PostReadAction)
			if err != nil {
				return nil, err
			}
			standing, who, err := h.Space.confirmations(ctx, actor, post)
			if err != nil {
				return nil, err
			}
			if standing == nil {
				return nil, shared.ErrConflict.WithDetail("posts.confirmation_not_asked")
			}
			out, _ := confirmationsOutput(standing, who, h.Space.Clock.Now()).(usecase.Output)
			return out, nil
		}),
	})
}

// RemindPost tells those of the audience who have not confirmed a post.
type RemindPost struct{ Space Space }

// Descriptor is the catalogue entry.
func (h RemindPost) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemindPostName, Permission: service.PermissionWriteItems,
		Summary:     "Tells each of a post's audience who has not confirmed it yet, once per call.",
		SideEffects: "Records a reminder for each and writes an audit entry naming how many, never whom.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit:       usecase.AuditDeclaration{Action: PostRemindedAction, TargetType: postTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			post, err := h.Space.writablePost(ctx, actor, id, PostRemindedAction)
			if err != nil {
				return nil, err
			}
			now := h.Space.Clock.Now()
			if post.State(now) != domain.PostPublished || post.ConfirmBy == nil {
				return nil, shared.ErrConflict.WithDetail("posts.confirmation_not_asked")
			}
			audience, err := h.Space.audience(ctx, actor, post)
			if err != nil {
				return nil, err
			}
			var confirmed []shared.ID
			err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				confirmed, err = h.Space.Posting.Posts.ConfirmedBy(ctx, id)
				return err
			})
			if err != nil {
				return nil, err
			}
			open := slices.DeleteFunc(audience, func(id shared.ID) bool { return slices.Contains(confirmed, id) })
			if len(open) == 0 {
				return nil, shared.ErrConflict.WithDetail("posts.nobody_to_remind")
			}
			err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if h.Space.Posting.Notify != nil {
					if err := h.Space.Posting.Notify.PostReminder(ctx, actor.TenantID, post, open); err != nil {
						return err
					}
				}
				return h.Space.record(ctx, actor, PostRemindedAction, postTarget, id,
					[]audit.Change{{Field: "reminded", Classification: audit.Open, To: len(open)}}, now)
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"reminded": len(open)}, nil
		}),
	})
}

// PostPublishElapsed is the POST_PUBLISH purpose's handler in the timer pass: a scheduled post whose
// moment came is published at that moment, a post moved further ahead arms its next leg, and one
// published, trashed or gone meanwhile is left as it is.
type PostPublishElapsed struct {
	Pages  repository.Pages
	Posts  repository.Posts
	Timers ReviewTimers
	Clock  clock.Clock
	// Published is told of each post the pass published, after its transaction; nil tells nobody.
	Published func(ctx context.Context, tenant shared.ID, post domain.Post) error
}

// Elapsed handles one fired publishing timer.
func (h PostPublishElapsed) Elapsed(ctx context.Context, _ appshared.ActorContext, timer calendar.Timer) error {
	post, err := h.Posts.FindForUpdate(ctx, timer.SubjectID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	page, err := h.Pages.Find(ctx, post.PageID)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && page.IsTrashed()) {
		return nil
	}
	if err != nil {
		return err
	}
	now := h.Clock.Now()
	post.TenantID = timer.TenantID
	if !post.IsDue(now) {
		if post.State(now) != domain.PostScheduled || h.Timers == nil {
			return nil
		}
		_, err := h.Timers.Arm(ctx, timer.TenantID, calendar.PurposePostPublish, post.PageID, postTimer,
			calendar.Continuous(), max(min(post.PublishAt.Sub(now), calendar.MaxBudget), time.Second))
		return err
	}
	post = post.Published(now)
	if err := h.Posts.Update(ctx, post); err != nil {
		return err
	}
	if h.Published == nil {
		return nil
	}
	return h.Published(ctx, timer.TenantID, post)
}
