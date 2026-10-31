// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const RateMyRequestName = "RateMyRequest"

// RequestRatedAction is a requester rating their resolved request. The score is an open value;
// the words are a comment, audited by AddComment.
const RequestRatedAction audit.Action = "request.rated"

// RatingOutput is the shape the portal answers (api/openapi.yaml, schema RequestSatisfaction), or
// nil for none.
func RatingOutput(rating *domain.Rating) any {
	if rating == nil {
		return nil
	}
	return usecase.Output{"score": rating.Score, "rated_at": rating.RatedAt.UTC()}
}

// RateMyRequestCommand is the input, typed.
type RateMyRequestCommand struct {
	ItemID  shared.ID
	Score   int
	Comment string
}

// RateMyRequest answers the one question on resolution (UC-SVC-07): how satisfied the requester
// was, from 1 to 5, with a comment if they like. Only the requester, only on their own request,
// and only once it is resolved; rating again replaces the score.
type RateMyRequest struct{ Portal Portal }

// Execute stores the score and, when there are words, adds them as the requester's comment.
func (h RateMyRequest) Execute(ctx context.Context, actor appshared.ActorContext, cmd RateMyRequestCommand) (domain.Rating, error) {
	item, err := h.Portal.ownRequest(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.Rating{}, err
	}
	if !item.Completion.IsCompleted {
		return domain.Rating{}, shared.ErrConflict.WithDetail("satisfaction.request_open")
	}
	now := h.Portal.Desk.Clock.Now()
	rating, err := domain.NewRating(h.Portal.Desk.IDs.NewID(), actor.TenantID, item.ID, cmd.Score, now)
	if err != nil {
		return domain.Rating{}, err
	}
	if comment := strings.TrimSpace(cmd.Comment); comment != "" {
		// Before the score, so that a comment refused - too long, say - leaves no score without
		// the words the requester meant to go with it.
		if _, err := h.Portal.Entries.Invoke(ctx, addCommentUseCase, portal(actor), usecase.Input{
			"item_id": item.ID.String(), "body": comment,
		}); err != nil {
			return domain.Rating{}, err
		}
	}
	err = h.Portal.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if rating, err = h.Portal.Desk.Ratings.Save(ctx, rating); err != nil {
			return err
		}
		return h.Portal.Desk.record(ctx, actor, RequestRatedAction, requestTarget, item.ID, now,
			audit.Change{Field: "score", Classification: audit.Open, To: rating.Score})
	})
	return rating, err
}

// Descriptor is the catalogue entry.
func (h RateMyRequest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RateMyRequestName, Permission: service.PermissionRequest,
		Summary: "Says how satisfied the caller was with a request they raised, once it is resolved: a score " +
			"from 1 (very dissatisfied) to 5 (very satisfied) and, if they like, a comment the team reads. " +
			"Rating again replaces the score; an open request is refused with satisfaction.request_open.",
		SideEffects: "Writes the score and an audit entry; a comment is written through AddComment on the PORTAL door.",
		TokenScope:  "comments:write",
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The request."},
			{Name: "score", Kind: usecase.KindInt, Required: true, Description: "1 to 5."},
			{Name: "comment", Kind: usecase.KindString, Description: "Words for the team, up to 20000 characters."},
		},
		Audit:    usecase.AuditDeclaration{Action: RequestRatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a score is the requester's answer about the entry, not a change of it; a comment is recorded by AddComment."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			rating, err := h.Execute(ctx, actor, RateMyRequestCommand{
				ItemID: itemID, Score: in.Int("score"), Comment: in.String("comment"),
			})
			if err != nil {
				return nil, err
			}
			out, _ := RatingOutput(&rating).(usecase.Output)
			return out, nil
		}),
	}, requestTarget)
}
