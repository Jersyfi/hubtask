// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PostRow is a post as a hub's list shows it, with what the asking account has done with it.
type PostRow struct {
	Post      knowledge.Post
	Title     string
	Icon      string
	CreatedBy shared.ID
	CreatedAt time.Time
	// SeenCount is how many have seen it; who is never answered.
	SeenCount int
	Seen      bool
	Confirmed bool
}

// PostFilter narrows a hub's posts: the account's own, those it has not seen, or one post.
type PostFilter struct {
	Mine   bool
	Unread bool
	PageID shared.ID
}

// Reply is one reply in a post's discussion; a taken-back one keeps its place without its words.
type Reply struct {
	ID        shared.ID
	PageID    shared.ID
	ParentID  shared.ID
	AuthorID  shared.ID
	Body      string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// ReactionCount is how many gave one reaction to a reply, and whether the asking account did.
type ReactionCount struct {
	ReplyID  shared.ID
	Reaction knowledge.Reaction
	Count    int
	Mine     bool
}

// Posts stores posts, who has seen and confirmed them, and their discussion (migration 0401).
// Every method runs inside the caller's transaction, under its tenant (rule 3); another workspace's
// post is not found. Who sees a post is not decided here (rule 2).
type Posts interface {
	Insert(ctx context.Context, post knowledge.Post) error
	// Find answers the post held by a page; not found for a page that is no post.
	Find(ctx context.Context, pageID shared.ID) (knowledge.Post, error)
	FindForUpdate(ctx context.Context, pageID shared.ID) (knowledge.Post, error)
	Update(ctx context.Context, post knowledge.Post) error
	// PostPages answers which of the pages named are posts.
	PostPages(ctx context.Context, ids []shared.ID) (map[shared.ID]bool, error)
	// List answers a hub's posts that are not trashed, featured first, then newest first.
	List(ctx context.Context, hubID, account shared.ID, filter PostFilter, now time.Time, limit int) ([]PostRow, error)
	MarkSeen(ctx context.Context, pageID, account shared.ID, at time.Time) error
	Confirm(ctx context.Context, pageID, account shared.ID, at time.Time) error
	// ConfirmedBy answers who confirmed the post, for its writers alone.
	ConfirmedBy(ctx context.Context, pageID shared.ID) ([]shared.ID, error)
	// PeopleReaching answers the people holding a role that may reach the hub - the workspace's,
	// for a zero hub -, bounded; whether each reads it is the application's to ask.
	PeopleReaching(ctx context.Context, hubID shared.ID, limit int) ([]shared.ID, error)
	// GroupsOf answers each account's groups.
	GroupsOf(ctx context.Context, accounts []shared.ID) (map[shared.ID][]shared.ID, error)
	InsertReply(ctx context.Context, reply Reply) error
	FindReply(ctx context.Context, id shared.ID) (Reply, error)
	Replies(ctx context.Context, pageID shared.ID, limit int) ([]Reply, error)
	TakeBackReply(ctx context.Context, id shared.ID, at time.Time) error
	Reactions(ctx context.Context, pageID, account shared.ID) ([]ReactionCount, error)
	React(ctx context.Context, replyID, account shared.ID, reaction knowledge.Reaction) error
	Unreact(ctx context.Context, replyID, account shared.ID, reaction knowledge.Reaction) error
}
