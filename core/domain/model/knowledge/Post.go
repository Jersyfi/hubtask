// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PostTemplate is one of a post's own starts beside the pages' templates (B-1).
type PostTemplate string

// The post's own templates.
const (
	PostAnnouncement  PostTemplate = "ANNOUNCEMENT"
	PostReleaseNote   PostTemplate = "RELEASE_NOTE"
	PostMonthlyReview PostTemplate = "MONTHLY_REVIEW"
	PostWelcome       PostTemplate = "WELCOME"
)

// PostTemplates is every post template in the order the writer is offered them.
var PostTemplates = []PostTemplate{PostAnnouncement, PostReleaseNote, PostMonthlyReview, PostWelcome}

// Audience is who of the hub's readers a post is for (B-6). It only narrows: nobody reads a post
// who may not read its hub.
type Audience string

const (
	// AudienceHub is every reader of the hub.
	AudienceHub Audience = "HUB"
	// AudienceGroups is the hub's readers who belong to one of the named groups.
	AudienceGroups Audience = "GROUPS"
)

// MaxAudienceGroups bounds the groups a post is narrowed to.
const MaxAudienceGroups = 20

// PostState is where a post stands at a moment (B-3).
type PostState string

const (
	// PostDraft has no publishing moment: only its writers see it.
	PostDraft PostState = "DRAFT"
	// PostScheduled is to be published at a moment still ahead: only its writers see it.
	PostScheduled PostState = "SCHEDULED"
	// PostPublished is out for its audience.
	PostPublished PostState = "PUBLISHED"
)

// Post is a page with a date (B-1): the page holds its words, cover, icon and labels; the post
// adds when it goes out, for whom, whether it stays on top, whether it is discussed and whether
// its readers are asked to confirm it.
type Post struct {
	PageID   shared.ID
	TenantID shared.ID
	HubID    shared.ID
	Template PostTemplate
	// PublishAt is when it goes out - now or a day and time ahead; nil is a draft.
	PublishAt *time.Time
	// PublishedAt is when the scheduler or the writer published it; nil until then.
	PublishedAt *time.Time
	// FeaturedUntil keeps it on top of the hub's posts until then; nil never.
	FeaturedUntil *time.Time
	Audience      Audience
	GroupIDs      []shared.ID
	// Discussion opens the discussion under the post (B-4).
	Discussion bool
	// Notify tells the hub's followers once it is published (B-5).
	Notify bool
	// ConfirmBy asks the audience to confirm they read it by then (B-5); nil asks nothing.
	ConfirmBy *time.Time
}

// PostSettings is what a writer asks of a post.
type PostSettings struct {
	Template      PostTemplate
	PublishAt     *time.Time
	FeaturedUntil *time.Time
	Audience      Audience
	GroupIDs      []shared.ID
	Discussion    bool
	Notify        bool
	ConfirmBy     *time.Time
}

// NewPostSettings checks what a writer asks: a known template (or none), an audience that names
// groups exactly when it is GROUPS, and a feature and a confirmation that end after it goes out.
func NewPostSettings(in PostSettings, now time.Time) (PostSettings, error) {
	if in.Template != "" && !slices.Contains(PostTemplates, in.Template) {
		return PostSettings{}, invalid("/template", "posts.template_unknown")
	}
	switch in.Audience {
	case "", AudienceHub:
		in.Audience = AudienceHub
		if len(in.GroupIDs) > 0 {
			return PostSettings{}, invalid("/group_ids", "posts.groups_without_audience")
		}
	case AudienceGroups:
		if len(in.GroupIDs) == 0 || len(in.GroupIDs) > MaxAudienceGroups {
			return PostSettings{}, invalid("/group_ids", "posts.groups_count")
		}
		in.GroupIDs = slices.Clone(in.GroupIDs)
		slices.Sort(in.GroupIDs)
		in.GroupIDs = slices.Compact(in.GroupIDs)
	default:
		return PostSettings{}, invalid("/audience", "posts.audience_unknown")
	}
	from := now
	if in.PublishAt != nil && in.PublishAt.After(now) {
		from = *in.PublishAt
	}
	if in.FeaturedUntil != nil && !in.FeaturedUntil.After(from) {
		return PostSettings{}, invalid("/featured_until", "posts.featured_until_past")
	}
	if in.ConfirmBy != nil && !in.ConfirmBy.After(from) {
		return PostSettings{}, invalid("/confirm_by", "posts.confirm_by_past")
	}
	return in, nil
}

// State answers where the post stands at now.
func (p Post) State(now time.Time) PostState {
	switch {
	case p.PublishedAt != nil:
		return PostPublished
	case p.PublishAt == nil:
		return PostDraft
	case p.PublishAt.After(now):
		return PostScheduled
	default:
		return PostPublished
	}
}

// IsDue answers whether the scheduler publishes it now: its moment has come and nobody published it.
func (p Post) IsDue(now time.Time) bool {
	return p.PublishedAt == nil && p.PublishAt != nil && !p.PublishAt.After(now)
}

// Published is the post published at now, at the moment it was asked for.
func (p Post) Published(now time.Time) Post {
	at := now
	if p.PublishAt != nil && p.PublishAt.Before(now) {
		at = *p.PublishAt
	}
	p.PublishedAt = &at
	if p.PublishAt == nil {
		p.PublishAt = &at
	}
	return p
}

// IsFeatured answers whether it stands on top of the hub's posts at now.
func (p Post) IsFeatured(now time.Time) bool {
	return p.State(now) == PostPublished && p.FeaturedUntil != nil && p.FeaturedUntil.After(now)
}

// Reader is what decides whether somebody sees a post, judged by the application layer at the
// hub: whether they read the hub, write in it, and which of the post's groups they belong to.
type Reader struct {
	ReadsHub  bool
	WritesHub bool
	Groups    []shared.ID
}

// Sees answers whether a reader sees the post at now (B-3, B-6): never without reading the hub; a
// draft or a scheduled post only for the hub's writers; a published one for its audience and its
// writers.
func (p Post) Sees(r Reader, now time.Time) bool {
	if !r.ReadsHub {
		return false
	}
	if r.WritesHub {
		return true
	}
	if p.State(now) != PostPublished {
		return false
	}
	if p.Audience != AudienceGroups {
		return true
	}
	for _, group := range r.Groups {
		if slices.Contains(p.GroupIDs, group) {
			return true
		}
	}
	return false
}

// Reaction is one of the few fixed reactions to a reply in a post's discussion (B-4).
type Reaction string

// The reactions.
const (
	ReactionThanks    Reaction = "THANKS"
	ReactionAgree     Reaction = "AGREE"
	ReactionCelebrate Reaction = "CELEBRATE"
	ReactionIdea      Reaction = "IDEA"
)

// Reactions is every reaction in the order they are offered.
var Reactions = []Reaction{ReactionThanks, ReactionAgree, ReactionCelebrate, ReactionIdea}

// NewReaction checks a reaction as asked for.
func NewReaction(value string) (Reaction, error) {
	if !slices.Contains(Reactions, Reaction(value)) {
		return "", invalid("/reaction", "posts.reaction_unknown")
	}
	return Reaction(value), nil
}

// Confirmations is how a post's read confirmation stands (B-5): shown to its writers only.
type Confirmations struct {
	Confirmed int
	Audience  int
	ConfirmBy time.Time
}

// MayRemind answers whether a writer may remind those who have not confirmed: while some have not.
func (c Confirmations) MayRemind() bool { return c.Confirmed < c.Audience }

// IsOverdue answers whether the deadline passed with somebody still to confirm.
func (c Confirmations) IsOverdue(now time.Time) bool {
	return c.MayRemind() && now.After(c.ConfirmBy)
}
