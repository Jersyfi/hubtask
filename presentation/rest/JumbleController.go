// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The jumble (G-10). The controller maps documents and nothing else: what may be submitted, who
// may read the inbox and what a conversion produces are all decided inwards of here (ADR-0005).

const (
	submitJumbleEntryUseCase  = "SubmitJumbleEntry"
	listJumbleEntriesUseCase  = "ListJumbleEntries"
	convertJumbleEntryUseCase = "ConvertJumbleEntry"
	dismissJumbleEntryUseCase = "DismissJumbleEntry"
)

// ConvertJumbleEntry answers POST /jumble/entries/{entryId}:convert.
func (c *RestController) ConvertJumbleEntry(
	w http.ResponseWriter, r *http.Request, entryID openapi_types.UUID,
	_ openapi.ConvertJumbleEntryParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.JumbleEntryConvert
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{
			"entry_id":      entryID.String(),
			"collection_id": body.CollectionId.String(),
		}
		if body.BucketId != nil {
			input["bucket_id"] = body.BucketId.String()
		}
		if body.Title != nil {
			input["title"] = *body.Title
		}
		if body.Type != nil {
			input["type"] = string(*body.Type)
		}
		return c.UseCases.Invoke(r.Context(), convertJumbleEntryUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, jumbleEntryResponse(out))
	})
}

// DismissJumbleEntry answers POST /jumble/entries/{entryId}:dismiss.
func (c *RestController) DismissJumbleEntry(
	w http.ResponseWriter, r *http.Request, entryID openapi_types.UUID,
	_ openapi.DismissJumbleEntryParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), dismissJumbleEntryUseCase, actor,
			usecase.Input{"entry_id": entryID.String()})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, jumbleEntryResponse(out))
	})
}

// ListJumbleEntries answers GET /jumble/entries.
func (c *RestController) ListJumbleEntries(
	w http.ResponseWriter, r *http.Request, params openapi.ListJumbleEntriesParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{}
		if params.Status != nil {
			input["status"] = string(*params.Status)
		}
		if params.Channel != nil {
			input["channel"] = string(*params.Channel)
		}
		if params.Cursor != nil {
			input["cursor"] = *params.Cursor
		}
		if params.Size != nil {
			input["size"] = *params.Size
		}
		return c.UseCases.Invoke(r.Context(), listJumbleEntriesUseCase, actor, input)
	}, func(out usecase.Output) {
		rows, _ := out["data"].([]usecase.Output)
		entries := make([]openapi.JumbleEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, jumbleEntryResponse(row))
		}
		writeJSON(w, r, http.StatusOK, openapi.JumbleEntryPage{
			Data: entries, Page: pageResponse(out),
		})
	})
}

// SubmitJumbleEntry answers POST /jumble/entries.
func (c *RestController) SubmitJumbleEntry(
	w http.ResponseWriter, r *http.Request, _ openapi.SubmitJumbleEntryParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.JumbleEntrySubmit
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{}
		if body.Channel != nil {
			input["channel"] = string(*body.Channel)
		}
		if body.Sender != nil {
			input["sender"] = *body.Sender
		}
		if body.RawSubject != nil {
			input["raw_subject"] = *body.RawSubject
		}
		if body.RawBody != nil {
			input["raw_body"] = *body.RawBody
		}
		if body.Attachments != nil {
			attachments := make([]any, 0, len(*body.Attachments))
			for _, id := range *body.Attachments {
				attachments = append(attachments, id.String())
			}
			input["attachments"] = attachments
		}
		return c.UseCases.Invoke(r.Context(), submitJumbleEntryUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusCreated, jumbleEntryResponse(out))
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
	return entry
}

// JumbleIntakeDeliverer serves the public intake route: not a catalogue entry, because it answers
// a credential nobody in this system holds (G-10). Nil leaves the route answering the pending 404.
type JumbleIntakeDeliverer interface {
	Deliver(ctx context.Context, presented, sender, subject, body string) (jumbledomain.Entry, error)
}

const rotateJumbleIntakeUseCase = "RotateJumbleIntake"

// RotateJumbleIntake answers POST /jumble/intake:rotate-token.
func (c *RestController) RotateJumbleIntake(
	w http.ResponseWriter, r *http.Request, _ openapi.RotateJumbleIntakeParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), rotateJumbleIntakeUseCase, actor, usecase.Input{})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, openapi.JumbleIntakeToken{
			Token:     out.String("token"),
			RotatedAt: timeValue(out["rotated_at"]),
		})
	})
}

// StartJumbleIntake answers POST /jumble/inbound/{token}.
func (c *RestController) StartJumbleIntake(
	w http.ResponseWriter, r *http.Request, presented string,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.JumbleIntake == nil {
		// The pending 404 rather than an internal error: an installation that does not serve this
		// tells the internet nothing about why (G-08's reasoning for the inbound route).
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
