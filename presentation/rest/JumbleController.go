// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The jumble. The controller maps documents and nothing else: what may be submitted, who
// may read the inbox and what a conversion produces are all decided inwards of here (ADR-0005).

// DeliverMail answers POST /jumble/mail/{token}.
//
// The body is read whole rather than streamed, and that is what the bound is for: the request
// middleware refuses a payload over the mail limit before a byte of it is read, so what reaches
// this is a message of a size this installation agreed to take (T-17). Reading it whole is what
// the parser needs - MIME is walked, not consumed - and it is the same shape a mail spool has.
func (c *RestController) DeliverMail(w http.ResponseWriter, r *http.Request, presented string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.MailIntake == nil {
		// The pending 404, for the webhook door's reason: an installation that does not serve this
		// tells the internet nothing about why.
		c.pending.DeliverMail(w, r, presented)
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		// A body that stopped short or ran past the bound. Malformed rather than internal: what
		// went wrong is the request, and the sender is the one who can do something about it.
		WriteProblem(w, shared.ErrMalformedRequest.
			WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}

	entry, err := c.MailIntake.Deliver(r.Context(), presented, raw)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, openapi.JumbleIntakeAccepted{
		EntryId: uuidValue(entry.ID.String()),
	})
}

func jumbleEntryResponse(out usecase.Output) openapi.JumbleEntry {
	attachments := []openapi_types.UUID{}
	if raw, ok := out["attachments"].([]any); ok {
		for _, id := range raw {
			if text, ok := id.(string); ok {
				attachments = append(attachments, uuidValue(text))
			}
		}
	}

	entry := openapi.JumbleEntry{
		Id:          uuidValue(out.String("id")),
		Channel:     openapi.JumbleEntryChannel(out.String("channel")),
		Attachments: attachments,
		Status:      openapi.JumbleEntryStatus(out.String("status")),
		ReceivedAt:  timeValue(out["received_at"]),
	}
	if sender := out.String("sender"); sender != "" {
		entry.Sender = &sender
	}
	if subject := out.String("raw_subject"); subject != "" {
		entry.RawSubject = &subject
	}
	if body, ok := out["raw_body"].(string); ok && body != "" {
		entry.RawBody = &body
	}
	if id := out.String("target_item_id"); id != "" {
		target := uuidValue(id)
		entry.TargetItemId = &target
	}
	if settled, ok := out["settled_at"].(time.Time); ok {
		entry.SettledAt = &settled
	}
	if id := out.String("arrival_job_id"); id != "" {
		job := uuidValue(id)
		entry.ArrivalJobId = &job
	}
	if kind := out.String("meant_for_kind"); kind != "" {
		meant := openapi.JumbleEntryMeantForKind(kind)
		entry.MeantForKind = &meant
		id := uuidValue(out.String("meant_for_id"))
		entry.MeantForId = &id
	}
	if reason := out.String("intake_reason"); reason != "" {
		value := openapi.MailReason(reason)
		entry.IntakeReason = &value
	}
	return entry
}

// JumbleIntakeDeliverer serves the public intake route: not a catalogue entry, because it answers
// a credential nobody in this system holds. Nil leaves the route answering the pending 404.
type JumbleIntakeDeliverer interface {
	Deliver(ctx context.Context, presented, sender, subject, body string) (jumbledomain.Entry, error)
}

// MailDeliverer serves the mail route: the message as it arrived, and the entry it became. Not a
// catalogue entry either, for the webhook door's reason - it answers a credential nobody in this
// system holds. Nil leaves the route answering the pending 404.
type MailDeliverer interface {
	Deliver(ctx context.Context, presented string, raw []byte) (jumbledomain.Entry, error)
}

// StartJumbleIntake answers POST /jumble/inbound/{token}.
func (c *RestController) StartJumbleIntake(
	w http.ResponseWriter, r *http.Request, presented string,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.JumbleIntake == nil {
		// The pending 404 rather than an internal error: an installation that does not serve this
		// tells the internet nothing about why (the inbound route's reasoning).
		c.pending.StartJumbleIntake(w, r, presented)
		return
	}

	var body openapi.JumbleIntakeDelivery
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	sender, subject, text := "", "", ""
	if body.Sender != nil {
		sender = *body.Sender
	}
	if body.Subject != nil {
		subject = *body.Subject
	}
	if body.Body != nil {
		text = *body.Body
	}

	entry, err := c.JumbleIntake.Deliver(r.Context(), presented, sender, subject, text)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, openapi.JumbleIntakeAccepted{
		EntryId: uuidValue(entry.ID.String()),
	})
}

// jumbleDestinationResponse is one collection an entry can be converted into, with its hub.
func jumbleDestinationResponse(row usecase.Output) openapi.JumbleDestination {
	return openapi.JumbleDestination{
		Id: uuidValue(row.String("id")), Name: row.String("name"),
		HubId: uuidValue(row.String("hub_id")), HubName: row.String("hub_name"),
		HubPrivate: row["hub_private"] == true,
	}
}

// jumbleIntakeTokenResponse is the one answer that carries the intake's token.
func jumbleIntakeTokenResponse(out usecase.Output) openapi.JumbleIntakeToken {
	return openapi.JumbleIntakeToken{Token: out.String("token"), RotatedAt: timeValue(out["rotated_at"])}
}
