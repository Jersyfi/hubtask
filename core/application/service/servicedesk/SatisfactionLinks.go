// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strconv"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// SatisfactionQuestionsConsumer identifies the subscriber; stable, RequesterRepliesConsumer's reason.
const SatisfactionQuestionsConsumer = "servicedesk-satisfaction-questions"

// RequestRatedByLinkAction is a requester without an account rating their request by the link of
// the resolution mail. The actor is the system: nobody signed in, and the address is not recorded
// in the trail (rule 10).
const RequestRatedByLinkAction audit.Action = "request.rated_by_link"

const (
	satisfactionQuestionSubject = "email.requester_satisfaction.subject"
	satisfactionQuestionBody    = "email.requester_satisfaction.body"
)

// SatisfactionQuestions queues the satisfaction question of a resolved entry filed for a
// requester without an account (UC-SVC-07 check 6): one job per completion, keyed by its event, so
// an event delivered twice mails once. It never takes a replay: a restore's completions were asked
// about when they happened.
type SatisfactionQuestions struct {
	Requesters RequesterAddresses
	Jobs       ReplyJobs
}

var _ eventbus.Subscriber = SatisfactionQuestions{}

// Name identifies the subscriber.
func (q SatisfactionQuestions) Name() string { return SatisfactionQuestionsConsumer }

// Wants reports whether an event can be a resolution.
func (q SatisfactionQuestions) Wants(eventType event.Type) bool {
	return eventType == event.ItemCompleted
}

// Deliver queues the question where the entry has requesters by mail.
func (q SatisfactionQuestions) Deliver(ctx context.Context, envelope event.Envelope) error {
	if envelope.Type != event.ItemCompleted {
		return nil
	}
	itemID := shared.ID(textAt(envelope.Payload, "id"))
	if itemID.IsZero() || envelope.ID.IsZero() {
		return nil
	}
	addresses, err := q.Requesters.AddressesOn(ctx, itemID)
	if err != nil || len(addresses) == 0 {
		return err
	}
	_, err = q.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindRequesterSatisfaction, TenantID: envelope.TenantID, DedupeKey: envelope.ID.String(),
		// The entry alone: whether it is still resolved and whom to ask is read when it is sent.
		Payload: map[string]any{"item_id": itemID.String()},
	})
	return err
}

// WorkspaceBase answers the address the workspace's links name - identity.WorkspaceAddress.
type WorkspaceBase interface {
	Of(ctx context.Context) (string, error)
}

// SendSatisfactionQuestion mails the satisfaction question of a resolved entry to each requester
// it was filed for by mail: one mail per address, each with its own link, which answers once and
// for SatisfactionLinkLife. The five scores are five addresses of the web app's rating page; the
// score rides in the fragment beside the token, so a mail scanner that opens them rates nothing -
// the page asks before it sends.
//
// The links are written in one transaction and the mails sent outside any. A retry after a lost
// send mails new links, which replace the old ones.
type SendSatisfactionQuestion struct {
	Items      workrepo.Items
	Requesters RequesterAddresses
	Keys       TicketKeys
	Links      repository.SatisfactionLinks
	Address    WorkspaceBase
	// Replies answers the entry's address, where words for the team go; nil sends no Reply-To.
	Replies ReplyAddress
	// Workspaces answers the workspace's language; nil uses FallbackLocale.
	Workspaces     WorkspaceLanguage
	FallbackLocale string
	Entropy        clock.Entropy
	Clock          clock.Clock
	Mail           mail.Sender
	Renderer       i18n.Renderer
	Background     persistence.UnitOfWork
}

// Send mails the question the job names. Nothing to ask - the entry gone, reopened, without
// requesters, or the workspace without an address a link could name - is done, not failed.
func (s SendSatisfactionQuestion) Send(ctx context.Context, tenantID, itemID shared.ID) error {
	var messages []mail.Message
	err := s.Background.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		var err error
		messages, err = s.compose(ctx, tenantID, itemID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, message := range messages {
		if err := s.Mail.Send(ctx, message); err != nil {
			return err
		}
	}
	return nil
}

func (s SendSatisfactionQuestion) compose(ctx context.Context, tenantID, itemID shared.ID) ([]mail.Message, error) {
	item, err := s.Items.Find(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.DeletedAt != nil || !item.Completion.IsCompleted {
		return nil, nil
	}
	to, err := s.Requesters.AddressesOn(ctx, item.ID)
	if err != nil || len(to) == 0 {
		return nil, err
	}
	base, err := s.Address.Of(ctx)
	if err != nil {
		return nil, err
	}
	if base == "" {
		return nil, nil
	}
	key, err := s.Keys.KeyOf(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	locale := s.FallbackLocale
	if s.Workspaces != nil {
		workspace, err := s.Workspaces.Find(ctx)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		if workspace.DefaultLocale != "" {
			locale = workspace.DefaultLocale
		}
	}
	replyTo := ""
	if s.Replies != nil {
		// A convenience, as on a reply: the ticket key in the subject routes the answer too.
		if replyTo, err = s.Replies.ReplyAddress(ctx, tenantID, item.ID); err != nil {
			replyTo = ""
		}
	}
	now := s.Clock.Now()
	messages := make([]mail.Message, 0, len(to))
	for _, requester := range to {
		link, err := domain.NewSatisfactionLink(tenantID, requester.ID, item.ID, now)
		if err != nil {
			return nil, err
		}
		secret, err := s.Entropy.Bytes(integration.SatisfactionLinkTokenSecretBytes)
		if err != nil {
			return nil, shared.ErrInternal.WithDetail("satisfaction.link_unmintable").WithCause(err)
		}
		token, err := integration.NewSatisfactionLinkToken(tenantID, secret)
		if err != nil {
			return nil, err
		}
		if err := s.Links.Put(ctx, link, token); err != nil {
			return nil, err
		}
		params := map[string]string{"key": key.String(), "title": oneLine(item.Title)}
		for score := domain.MinSatisfaction; score <= domain.MaxSatisfaction; score++ {
			params["link_"+strconv.Itoa(score)] = SatisfactionAddress(base, token, score)
		}
		messages = append(messages, mail.Message{
			To: requester.Address, ReplyTo: replyTo,
			Subject: s.Renderer.Render(locale, satisfactionQuestionSubject, params),
			Body:    s.Renderer.Render(locale, satisfactionQuestionBody, params),
		})
	}
	return messages, nil
}

// SatisfactionAddress is the web app's rating page for one score: `<base>/rate#<token>.<score>`.
// The fragment never reaches a server log or a Referer.
func SatisfactionAddress(base string, token integration.SatisfactionLinkToken, score int) string {
	return strings.TrimRight(base, "/") + "/rate#" + token.Secret() + "." + strconv.Itoa(score)
}

// AnswerSatisfactionLink records the score a requester without an account gave by the link of a
// resolution mail (UC-SVC-07 check 6). Not a use case and not in the catalogue, OpenPublicForm's
// reason: it answers a credential nobody in the system holds, and the tenant comes from the token.
//
// The score is the request's rating, the portal's (one per request, replacing an earlier one); the
// link answers once and within its life. A score off the scale is refused before the link is
// spent, so a corrected answer still goes through.
type AnswerSatisfactionLink struct {
	Links      repository.SatisfactionLinks
	Items      workrepo.Items
	Ratings    repository.Ratings
	Audit      audit.Sink
	IDs        clock.IDGenerator
	Clock      clock.Clock
	UnitOfWork persistence.UnitOfWork
}

func satisfactionLinkGone() error {
	return shared.ErrNotFound.WithDetail("satisfaction.link_not_found")
}

// Execute records the score.
func (h AnswerSatisfactionLink) Execute(ctx context.Context, token integration.SatisfactionLinkToken, score int) error {
	if token.IsZero() {
		return satisfactionLinkGone()
	}
	now := h.Clock.Now()
	scope := persistence.Scope{TenantID: token.TenantID()}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		link, err := h.Links.FindByToken(ctx, token)
		if err != nil {
			return err
		}
		if _, err := link.Use(now); err != nil {
			return err
		}
		item, err := h.Items.Find(ctx, link.ItemID)
		if err != nil {
			return err
		}
		if item.DeletedAt != nil {
			return satisfactionLinkGone()
		}
		if !item.Completion.IsCompleted {
			return shared.ErrConflict.WithDetail("satisfaction.request_open")
		}
		rating, err := domain.NewRating(h.IDs.NewID(), token.TenantID(), item.ID, score, now)
		if err != nil {
			return err
		}
		used, err := h.Links.Use(ctx, token, now)
		if err != nil {
			return err
		}
		if !used {
			return shared.ErrConflict.WithDetail("satisfaction.link_used")
		}
		if rating, err = h.Ratings.Save(ctx, rating); err != nil {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: token.TenantID(), OccurredAt: now, Action: RequestRatedByLinkAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: appshared.ActorSystem, TargetType: requestTarget, TargetID: item.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{Field: "score", Classification: audit.Open, To: rating.Score}),
		})
	})
	if errors.Is(err, shared.ErrNotFound) {
		return satisfactionLinkGone()
	}
	return err
}
