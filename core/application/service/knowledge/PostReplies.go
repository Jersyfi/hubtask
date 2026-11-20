// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// maxReplyBytes bounds a reply as the column does.
const maxReplyBytes = 65536

// ReplyOutput is the shape every door returns (api/openapi.yaml, schema PostReply).
func ReplyOutput(reply repository.Reply, reactions []repository.ReactionCount) usecase.Output {
	counted := []any{}
	for _, r := range reactions {
		if r.ReplyID == reply.ID {
			counted = append(counted, usecase.Output{"reaction": string(r.Reaction), "count": r.Count, "mine": r.Mine})
		}
	}
	return usecase.Output{
		"id": reply.ID.String(), "page_id": reply.PageID.String(), "parent_id": idOrNil(reply.ParentID),
		"author_id": idOrNil(reply.AuthorID), "body": reply.Body, "created_at": reply.CreatedAt,
		"deleted": reply.DeletedAt != nil, "reactions": counted,
	}
}

// discussed reads a published post the actor sees, whose discussion is open.
func (s Space) discussed(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, action audit.Action) (repository.PostRow, domain.Reader, error) {
	row, reader, err := s.visiblePost(ctx, actor, pageID, action)
	if err != nil {
		return row, reader, err
	}
	if row.Post.State(s.Clock.Now()) != domain.PostPublished {
		return row, reader, shared.ErrConflict.WithDetail("posts.not_published")
	}
	if !row.Post.Discussion {
		return row, reader, shared.ErrConflict.WithDetail("posts.discussion_closed")
	}
	return row, reader, nil
}

// replyOf reads one reply of the post with its reactions as the actor sees them.
func (s Space) replyOf(ctx context.Context, actor appshared.ActorContext, pageID, replyID shared.ID) (usecase.Output, error) {
	var reply repository.Reply
	var reactions []repository.ReactionCount
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if reply, err = s.Posting.Posts.FindReply(ctx, replyID); err != nil {
			return err
		}
		reactions, err = s.Posting.Posts.Reactions(ctx, pageID, person(actor))
		return err
	})
	if err != nil {
		return nil, err
	}
	if reply.PageID != pageID {
		return nil, shared.ErrNotFound.WithDetail("posts.reply_not_found")
	}
	return ReplyOutput(reply, reactions), nil
}

// ListPostReplies reads a post's discussion.
type ListPostReplies struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ListPostReplies) Descriptor() usecase.Descriptor {
	return postDescriptor(usecase.Descriptor{
		Name: ListPostRepliesName, Permission: service.PermissionRead,
		Summary: "Reads the replies under a post, oldest first, one level deep, each with its " +
			"reactions counted and whether the caller gave them.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."}},
		Audit: usecase.AuditDeclaration{Action: PostReadAction, TargetType: postTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			if _, _, err := h.Space.visiblePost(ctx, actor, id, PostReadAction); err != nil {
				return nil, err
			}
			var replies []repository.Reply
			var reactions []repository.ReactionCount
			err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if replies, err = h.Space.Posting.Posts.Replies(ctx, id, maxReplies); err != nil {
					return err
				}
				reactions, err = h.Space.Posting.Posts.Reactions(ctx, id, person(actor))
				return err
			})
			if err != nil {
				return nil, err
			}
			data := make([]any, 0, len(replies))
			for _, reply := range replies {
				data = append(data, ReplyOutput(reply, reactions))
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// AddPostReply writes a reply under a post.
type AddPostReply struct{ Space Space }

// Descriptor is the catalogue entry.
func (h AddPostReply) Descriptor() usecase.Descriptor {
	return postDescriptor(usecase.Descriptor{
		Name: AddPostReplyName, Permission: service.PermissionRead,
		Summary: "Adds a reply under a published post whose discussion is open, or an answer to a " +
			"reply; anybody the post is for may reply.",
		SideEffects: "Writes the reply and an audit entry with its words as a fingerprint.",
		TokenScope:  pagesRead,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."},
			{Name: "body", Kind: usecase.KindString, Required: true, Description: "The reply in Markdown."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "The reply it answers."},
		},
		Audit: usecase.AuditDeclaration{Action: PostRepliedAction, TargetType: replyTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			parent, err := optionalID(in, "parent_id")
			if err != nil {
				return nil, err
			}
			if _, _, err := h.Space.discussed(ctx, actor, id, PostRepliedAction); err != nil {
				return nil, err
			}
			body, err := domain.CheckBody(in.String("body"), h.Space.Text)
			if err != nil {
				return nil, err
			}
			if body == "" || len(body) > maxReplyBytes {
				return nil, shared.ErrValidation.WithDetail("posts.reply_length").
					WithFields(shared.FieldError{Path: "/body", Code: "posts.reply_length"})
			}
			now := h.Space.Clock.Now()
			reply := repository.Reply{ID: h.Space.IDs.NewID(), PageID: id, ParentID: parent, AuthorID: actor.AccountID, Body: body, CreatedAt: now}
			err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if !parent.IsZero() {
					above, err := h.Space.Posting.Posts.FindReply(ctx, parent)
					if err != nil {
						return err
					}
					// One level of replies: an answer goes under the reply that opened the thread.
					if above.PageID != id || !above.ParentID.IsZero() {
						return shared.ErrValidation.WithDetail("posts.reply_parent").
							WithFields(shared.FieldError{Path: "/parent_id", Code: "posts.reply_parent"})
					}
				}
				if err := h.Space.Posting.Posts.InsertReply(ctx, reply); err != nil {
					return err
				}
				return h.Space.record(ctx, actor, PostRepliedAction, replyTarget, reply.ID,
					[]audit.Change{{Field: "body", Classification: audit.Sensitive, To: body}}, now)
			})
			if err != nil {
				return nil, err
			}
			return ReplyOutput(reply, nil), nil
		}),
	})
}

// DeletePostReply takes a reply back: its author, or a writer of the hub.
type DeletePostReply struct{ Space Space }

// Descriptor is the catalogue entry.
func (h DeletePostReply) Descriptor() usecase.Descriptor {
	return postDescriptor(usecase.Descriptor{
		Name: DeletePostReplyName, Permission: service.PermissionRead,
		Summary:     "Takes a reply back: the caller's own, or any for a writer of the hub. Its place stays, its words go.",
		SideEffects: "Empties the reply and writes an audit entry.",
		TokenScope:  pagesRead,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."},
			{Name: "reply_id", Kind: usecase.KindID, Required: true, Description: "The reply."},
		},
		Audit: usecase.AuditDeclaration{Action: PostReplyGoneAction, TargetType: replyTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			replyID, err := in.ID("reply_id")
			if err != nil {
				return nil, err
			}
			_, reader, err := h.Space.visiblePost(ctx, actor, id, PostReplyGoneAction)
			if err != nil {
				return nil, err
			}
			now := h.Space.Clock.Now()
			err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reply, err := h.Space.Posting.Posts.FindReply(ctx, replyID)
				if err != nil {
					return err
				}
				if reply.PageID != id {
					return shared.ErrNotFound.WithDetail("posts.reply_not_found")
				}
				if reply.AuthorID != actor.AccountID && !reader.WritesHub {
					return shared.ErrForbidden.WithDetail("posts.reply_not_yours")
				}
				if err := h.Space.Posting.Posts.TakeBackReply(ctx, replyID, now); err != nil {
					return err
				}
				return h.Space.record(ctx, actor, PostReplyGoneAction, replyTarget, replyID, nil, now)
			})
			return nil, err
		}),
	})
}

// reaction is ReactToPostReply's and UnreactToPostReply's shared work.
func (s Space) reaction(ctx context.Context, actor appshared.ActorContext, in usecase.Input, give bool) (usecase.Output, error) {
	id, err := in.ID("page_id")
	if err != nil {
		return nil, err
	}
	replyID, err := in.ID("reply_id")
	if err != nil {
		return nil, err
	}
	reaction, err := domain.NewReaction(in.String("reaction"))
	if err != nil {
		return nil, err
	}
	if _, _, err := s.discussed(ctx, actor, id, PostReactedAction); err != nil {
		return nil, err
	}
	if !isPerson(actor) {
		return nil, shared.ErrForbidden.WithDetail("posts.person_reacts")
	}
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		reply, err := s.Posting.Posts.FindReply(ctx, replyID)
		if err != nil {
			return err
		}
		if reply.PageID != id || reply.DeletedAt != nil {
			return shared.ErrNotFound.WithDetail("posts.reply_not_found")
		}
		if give {
			return s.Posting.Posts.React(ctx, replyID, actor.AccountID, reaction)
		}
		return s.Posting.Posts.Unreact(ctx, replyID, actor.AccountID, reaction)
	})
	if err != nil {
		return nil, err
	}
	return s.replyOf(ctx, actor, id, replyID)
}

func reactionInput() []usecase.Field {
	kinds := make([]string, 0, len(domain.Reactions))
	for _, r := range domain.Reactions {
		kinds = append(kinds, string(r))
	}
	return []usecase.Field{
		{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The post's page."},
		{Name: "reply_id", Kind: usecase.KindID, Required: true, Description: "The reply."},
		{Name: "reaction", Kind: usecase.KindString, Required: true, Enum: kinds, Description: "One of the few fixed reactions."},
	}
}

// ReactToPostReply gives a reaction to a reply.
type ReactToPostReply struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReactToPostReply) Descriptor() usecase.Descriptor {
	return postDescriptor(usecase.Descriptor{
		Name: ReactToPostReplyName, Permission: service.PermissionRead,
		Summary:     "Gives one of the few fixed reactions to a reply in a post's discussion, once per person and kind.",
		SideEffects: "Records the reaction; no audit entry, since who reacted is personal.",
		TokenScope:  pagesRead,
		Input:       reactionInput(),
		Audit:       usecase.AuditDeclaration{Action: PostReactedAction, TargetType: replyTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Space.reaction(ctx, actor, in, true)
		}),
	})
}

// UnreactToPostReply takes a reaction back.
type UnreactToPostReply struct{ Space Space }

// Descriptor is the catalogue entry.
func (h UnreactToPostReply) Descriptor() usecase.Descriptor {
	return postDescriptor(usecase.Descriptor{
		Name: UnreactToPostReplyName, Permission: service.PermissionRead,
		Summary:     "Takes the caller's reaction of a kind to a reply back.",
		SideEffects: "Removes the reaction.",
		TokenScope:  pagesRead,
		Input:       reactionInput(),
		Audit:       usecase.AuditDeclaration{Action: PostReactedAction, TargetType: replyTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Space.reaction(ctx, actor, in, false)
		}),
	})
}
