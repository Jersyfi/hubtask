// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PostRepository stores posts, who has seen and confirmed them, and their discussion (migration
// 0401), through the unit of work's transaction (rule 3).
type PostRepository struct{}

// NewPostRepository answers the store.
func NewPostRepository() PostRepository { return PostRepository{} }

var _ repository.Posts = PostRepository{}

// postRow is the columns every read of a post answers.
type postRow struct {
	PageID, HubID                         pgtype.UUID
	Template                              *string
	PublishAt, PublishedAt, FeaturedUntil pgtype.Timestamptz
	Audience                              string
	GroupIds                              []pgtype.UUID //nolint:revive // sqlc's spelling, converted from its rows
	Discussion, Notify                    bool
	ConfirmBy                             pgtype.Timestamptz
}

func postFrom(row postRow) (knowledge.Post, error) {
	ids, err := pageIDs(row.PageID, row.HubID)
	if err != nil {
		return knowledge.Post{}, err
	}
	groups, err := idsFrom(row.GroupIds)
	if err != nil {
		return knowledge.Post{}, err
	}
	post := knowledge.Post{
		PageID: ids[0], HubID: ids[1], PublishAt: optionalTime(row.PublishAt),
		PublishedAt: optionalTime(row.PublishedAt), FeaturedUntil: optionalTime(row.FeaturedUntil),
		Audience: knowledge.Audience(row.Audience), GroupIDs: groups, Discussion: row.Discussion,
		Notify: row.Notify, ConfirmBy: optionalTime(row.ConfirmBy),
	}
	if row.Template != nil {
		post.Template = knowledge.PostTemplate(*row.Template)
	}
	return post, nil
}

// groupKeys is a post's groups as the column takes them: never null.
func groupKeys(ids []shared.ID) ([]pgtype.UUID, error) {
	keys, err := uuidsOf(ids)
	if keys == nil {
		keys = []pgtype.UUID{}
	}
	return keys, err
}

// Insert writes a new post for its page.
func (PostRepository) Insert(ctx context.Context, post knowledge.Post) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(post.PageID, post.HubID)
	if err != nil {
		return err
	}
	groups, err := groupKeys(post.GroupIDs)
	if err != nil {
		return err
	}
	if err := queries.InsertPost(ctx, sqlc.InsertPostParams{
		PageID: ids[0], HubID: ids[1], Template: optionalText(string(post.Template)),
		PublishAt: optionalTimestamp(post.PublishAt), PublishedAt: optionalTimestamp(post.PublishedAt),
		FeaturedUntil: optionalTimestamp(post.FeaturedUntil), Audience: string(post.Audience),
		GroupIds: groups, Discussion: post.Discussion, Notify: post.Notify,
		ConfirmBy: optionalTimestamp(post.ConfirmBy),
	}); err != nil {
		return pageFailed("writing the post", err)
	}
	return nil
}

// Find answers the post a page holds.
func (r PostRepository) Find(ctx context.Context, pageID shared.ID) (knowledge.Post, error) {
	return r.find(ctx, pageID, false)
}

// FindForUpdate answers the post a page holds and locks it.
func (r PostRepository) FindForUpdate(ctx context.Context, pageID shared.ID) (knowledge.Post, error) {
	return r.find(ctx, pageID, true)
}

func (PostRepository) find(ctx context.Context, pageID shared.ID, lock bool) (knowledge.Post, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Post{}, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return knowledge.Post{}, err
	}
	var row postRow
	if lock {
		var found sqlc.FindPostForUpdateRow
		found, err = queries.FindPostForUpdate(ctx, key)
		row = postRow(found)
	} else {
		var found sqlc.FindPostRow
		found, err = queries.FindPost(ctx, key)
		row = postRow(found)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return knowledge.Post{}, shared.ErrNotFound.WithDetail("posts.not_found")
	}
	if err != nil {
		return knowledge.Post{}, pageFailed("reading the post", err)
	}
	return postFrom(row)
}

// Update writes a post's settings and its publishing moment.
func (PostRepository) Update(ctx context.Context, post knowledge.Post) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(post.PageID)
	if err != nil {
		return err
	}
	groups, err := groupKeys(post.GroupIDs)
	if err != nil {
		return err
	}
	rows, err := queries.UpdatePost(ctx, sqlc.UpdatePostParams{
		PageID: key, Template: optionalText(string(post.Template)),
		PublishAt: optionalTimestamp(post.PublishAt), PublishedAt: optionalTimestamp(post.PublishedAt),
		FeaturedUntil: optionalTimestamp(post.FeaturedUntil), Audience: string(post.Audience),
		GroupIds: groups, Discussion: post.Discussion, Notify: post.Notify,
		ConfirmBy: optionalTimestamp(post.ConfirmBy),
	})
	if err != nil {
		return pageFailed("writing the post", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("posts.not_found")
	}
	return nil
}

// PostPages answers which of the pages named are posts.
func (PostRepository) PostPages(ctx context.Context, ids []shared.ID) (map[shared.ID]bool, error) {
	out := map[shared.ID]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PostPageIDs(ctx, keys)
	if err != nil {
		return nil, pageFailed("reading which pages are posts", err)
	}
	found, err := idsFrom(rows)
	if err != nil {
		return nil, err
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

// List answers a hub's posts that are not trashed.
func (PostRepository) List(
	ctx context.Context, hubID, account shared.ID, filter repository.PostFilter, now time.Time, limit int,
) ([]repository.PostRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := optionalUUIDs(hubID, account, filter.PageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListHubPosts(ctx, sqlc.ListHubPostsParams{
		HubID: ids[0], AccountID: ids[1], Mine: filter.Mine, Unread: filter.Unread, PageID: ids[2],
		PublishedOnly: filter.Published,
		Now:           timestampOf(now), Batch: boundedInt32(min(limit, 1000)),
	})
	if err != nil {
		return nil, pageFailed("reading the hub's posts", err)
	}
	out := make([]repository.PostRow, 0, len(rows))
	for _, row := range rows {
		post, err := postFrom(postRow{
			PageID: row.PageID, HubID: row.HubID, Template: row.Template, PublishAt: row.PublishAt,
			PublishedAt: row.PublishedAt, FeaturedUntil: row.FeaturedUntil, Audience: row.Audience,
			GroupIds: row.GroupIds, Discussion: row.Discussion, Notify: row.Notify, ConfirmBy: row.ConfirmBy,
		})
		if err != nil {
			return nil, err
		}
		author, err := optionalID(row.CreatedBy)
		if err != nil {
			return nil, err
		}
		cover, err := pageCoverFrom(row.Cover)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.PostRow{
			Post: post, Title: row.Title, Icon: row.Icon, CreatedBy: author, CreatedAt: row.CreatedAt.Time.UTC(),
			SeenCount: int(row.SeenCount), Seen: row.Seen, Confirmed: row.Confirmed,
			Cover: cover, Lead: row.Lead, Words: int(row.Words), Labels: row.Labels, Replies: int(row.ReplyCount),
		})
	}
	return out, nil
}

// MarkSeen counts the account among those who have seen the post, once.
func (PostRepository) MarkSeen(ctx context.Context, pageID, account shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID, account)
	if err != nil {
		return err
	}
	if err := queries.MarkPostSeen(ctx, sqlc.MarkPostSeenParams{PageID: ids[0], AccountID: ids[1], At: timestampOf(at)}); err != nil {
		return pageFailed("recording a post seen", err)
	}
	return nil
}

// Confirm records the account's read confirmation, once.
func (PostRepository) Confirm(ctx context.Context, pageID, account shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID, account)
	if err != nil {
		return err
	}
	if err := queries.ConfirmPost(ctx, sqlc.ConfirmPostParams{PageID: ids[0], AccountID: ids[1], At: timestampOf(at)}); err != nil {
		return pageFailed("recording a read confirmation", err)
	}
	return nil
}

// ConfirmedBy answers who confirmed the post.
func (PostRepository) ConfirmedBy(ctx context.Context, pageID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PostConfirmedBy(ctx, key)
	if err != nil {
		return nil, pageFailed("reading the read confirmations", err)
	}
	return idsFrom(rows)
}

// PeopleReaching answers the people holding a role that may reach the hub.
func (PostRepository) PeopleReaching(ctx context.Context, hubID shared.ID, limit int) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := optionalUUID(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PeopleReachingHub(ctx, sqlc.PeopleReachingHubParams{HubID: hub, Batch: boundedInt32(min(limit, 100000))})
	if err != nil {
		return nil, pageFailed("reading the people reaching a hub", err)
	}
	return idsFrom(rows)
}

// GroupsOf answers each account's groups.
func (PostRepository) GroupsOf(ctx context.Context, accounts []shared.ID) (map[shared.ID][]shared.ID, error) {
	out := map[shared.ID][]shared.ID{}
	if len(accounts) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(accounts)
	if err != nil {
		return nil, err
	}
	rows, err := queries.GroupsOfAccounts(ctx, keys)
	if err != nil {
		return nil, pageFailed("reading the groups of accounts", err)
	}
	for _, row := range rows {
		ids, err := pageIDs(row.AccountID, row.GroupID)
		if err != nil {
			return nil, err
		}
		out[ids[0]] = append(out[ids[0]], ids[1])
	}
	return out, nil
}

// InsertReply writes a reply in a post's discussion.
func (PostRepository) InsertReply(ctx context.Context, reply repository.Reply) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(reply.ID, reply.PageID, reply.AuthorID)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(reply.ParentID)
	if err != nil {
		return err
	}
	if err := queries.InsertPostReply(ctx, sqlc.InsertPostReplyParams{
		ID: ids[0], PageID: ids[1], ParentID: parent, AuthorID: ids[2], Body: reply.Body, At: timestampOf(reply.CreatedAt),
	}); err != nil {
		return pageFailed("writing a reply", err)
	}
	return nil
}

type replyRow struct {
	ID, PageID, ParentID, AuthorID pgtype.UUID
	Body                           string
	CreatedAt, DeletedAt           pgtype.Timestamptz
}

func replyFrom(row replyRow) (repository.Reply, error) {
	ids, err := pageIDs(row.ID, row.PageID)
	if err != nil {
		return repository.Reply{}, err
	}
	parent, err := optionalID(row.ParentID)
	if err != nil {
		return repository.Reply{}, err
	}
	author, err := optionalID(row.AuthorID)
	if err != nil {
		return repository.Reply{}, err
	}
	return repository.Reply{
		ID: ids[0], PageID: ids[1], ParentID: parent, AuthorID: author, Body: row.Body,
		CreatedAt: row.CreatedAt.Time.UTC(), DeletedAt: optionalTime(row.DeletedAt),
	}, nil
}

// FindReply answers one reply.
func (PostRepository) FindReply(ctx context.Context, id shared.ID) (repository.Reply, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Reply{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.Reply{}, err
	}
	row, err := queries.FindPostReply(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Reply{}, shared.ErrNotFound.WithDetail("posts.reply_not_found")
	}
	if err != nil {
		return repository.Reply{}, pageFailed("reading a reply", err)
	}
	return replyFrom(replyRow(row))
}

// Replies answers a post's discussion, oldest first.
func (PostRepository) Replies(ctx context.Context, pageID shared.ID, limit int) ([]repository.Reply, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPostReplies(ctx, sqlc.ListPostRepliesParams{PageID: key, Batch: boundedInt32(min(limit, 5000))})
	if err != nil {
		return nil, pageFailed("reading the discussion", err)
	}
	out := make([]repository.Reply, 0, len(rows))
	for _, row := range rows {
		reply, err := replyFrom(replyRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, reply)
	}
	return out, nil
}

// TakeBackReply empties a reply, keeping its place.
func (PostRepository) TakeBackReply(ctx context.Context, id shared.ID, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	rows, err := queries.TakeBackPostReply(ctx, sqlc.TakeBackPostReplyParams{ID: key, At: timestampOf(at)})
	if err != nil {
		return pageFailed("taking a reply back", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("posts.reply_not_found")
	}
	return nil
}

// Reactions answers the reactions to a post's replies, counted.
func (PostRepository) Reactions(ctx context.Context, pageID, account shared.ID) ([]repository.ReactionCount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := pageUUIDs(pageID, account)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PostReplyReactions(ctx, sqlc.PostReplyReactionsParams{PageID: ids[0], AccountID: ids[1]})
	if err != nil {
		return nil, pageFailed("reading the reactions", err)
	}
	out := make([]repository.ReactionCount, 0, len(rows))
	for _, row := range rows {
		reply, err := idFrom(row.ReplyID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.ReactionCount{
			ReplyID: reply, Reaction: knowledge.Reaction(row.Reaction), Count: int(row.Count), Mine: row.Mine,
		})
	}
	return out, nil
}

// React gives a reaction to a reply, once per account and kind.
func (PostRepository) React(ctx context.Context, replyID, account shared.ID, reaction knowledge.Reaction) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(replyID, account)
	if err != nil {
		return err
	}
	if err := queries.InsertPostReaction(ctx, sqlc.InsertPostReactionParams{
		ReplyID: ids[0], AccountID: ids[1], Reaction: string(reaction),
	}); err != nil {
		return pageFailed("writing a reaction", err)
	}
	return nil
}

// Unreact takes the account's reaction of a kind back.
func (PostRepository) Unreact(ctx context.Context, replyID, account shared.ID, reaction knowledge.Reaction) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(replyID, account)
	if err != nil {
		return err
	}
	if _, err := queries.DeletePostReaction(ctx, sqlc.DeletePostReactionParams{
		ReplyID: ids[0], AccountID: ids[1], Reaction: string(reaction),
	}); err != nil {
		return pageFailed("taking a reaction back", err)
	}
	return nil
}
