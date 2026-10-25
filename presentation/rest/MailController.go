// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Mail addresses for every place (ADR-0092). The controller maps documents; who may see or
// change an address, and where a delivery goes, are decided inwards of here.

const (
	showMailAddressUseCase     = "ShowMailAddress"
	rotateMailAddressUseCase   = "RotateMailAddress"
	setMailSenderPolicyUseCase = "SetMailSenderPolicy"
	commentJumbleEntryUseCase  = "CommentJumbleEntry"
)

// AddressedMailDeliverer serves the receiving domain's inbound route. Not a catalogue entry: it
// answers the installation's inbound key, which no person holds.
type AddressedMailDeliverer interface {
	Authorized(header string) bool
	Deliver(ctx context.Context, recipient string, raw []byte) (jumbleservice.MailDeliveryResult, error)
}

// ShowMailAddress answers GET /mail/addresses.
func (c *RestController) ShowMailAddress(
	w http.ResponseWriter, r *http.Request, params openapi.ShowMailAddressParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		input := usecase.Input{"target_kind": string(params.TargetKind)}
		if params.TargetId != nil {
			input["target_id"] = params.TargetId.String()
		}
		return c.UseCases.Invoke(r.Context(), showMailAddressUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, mailAddressResponse(out))
	})
}

// RotateMailAddress answers POST /mail/addresses:rotate.
func (c *RestController) RotateMailAddress(
	w http.ResponseWriter, r *http.Request, _ openapi.RotateMailAddressParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.MailAddressTarget
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		input := usecase.Input{"target_kind": string(body.TargetKind)}
		if body.TargetId != nil {
			input["target_id"] = body.TargetId.String()
		}
		return c.UseCases.Invoke(r.Context(), rotateMailAddressUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, mailAddressResponse(out))
	})
}

// SetMailSenderPolicy answers POST /mail/addresses:set-sender-policy.
func (c *RestController) SetMailSenderPolicy(
	w http.ResponseWriter, r *http.Request, _ openapi.SetMailSenderPolicyParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.MailSenderPolicyChange
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		input := usecase.Input{
			"target_kind": string(body.TargetKind), "sender_policy": string(body.SenderPolicy),
		}
		if body.TargetId != nil {
			input["target_id"] = body.TargetId.String()
		}
		return c.UseCases.Invoke(r.Context(), setMailSenderPolicyUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, mailAddressResponse(out))
	})
}

// CommentJumbleEntry answers POST /jumble/entries/{entryId}:comment.
func (c *RestController) CommentJumbleEntry(
	w http.ResponseWriter, r *http.Request, entryID openapi_types.UUID,
	_ openapi.CommentJumbleEntryParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.JumbleEntryComment
		if r.ContentLength != 0 {
			if err := decodeJSON(r, &body); err != nil {
				return nil, err
			}
		}
		input := usecase.Input{"entry_id": entryID.String()}
		if body.ItemId != nil {
			input["item_id"] = body.ItemId.String()
		}
		return c.UseCases.Invoke(r.Context(), commentJumbleEntryUseCase, actor, input)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, jumbleEntryResponse(out))
	})
}

// DeliverAddressedMail answers POST /mail/inbound.
//
// The body is read whole, the mail door's way: the request middleware bounds it before a byte is
// read. A missing or wrong key answers the same not-found an unknown address gets, so the route
// tells a stranger nothing about whether it exists.
func (c *RestController) DeliverAddressedMail(
	w http.ResponseWriter, r *http.Request, params openapi.DeliverAddressedMailParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.AddressedMail == nil || !c.AddressedMail.Authorized(r.Header.Get("Authorization")) {
		c.pending.DeliverAddressedMail(w, r, params)
		return
	}

	recipient := ""
	if params.Recipient != nil {
		recipient = *params.Recipient
	}
	var raw []byte
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType == "multipart/form-data" {
		fields, err := mimeFields(r, "body-mime", "recipient")
		if err != nil {
			WriteProblem(w, shared.ErrMalformedRequest.
				WithDetail("request.body_unreadable").WithCause(err), requestID)
			return
		}
		raw = []byte(fields["body-mime"])
		if recipient == "" {
			recipient = fields["recipient"]
		}
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteProblem(w, shared.ErrMalformedRequest.
				WithDetail("request.body_unreadable").WithCause(err), requestID)
			return
		}
		raw = body
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable"), requestID)
		return
	}

	result, err := c.AddressedMail.Deliver(r.Context(), recipient, raw)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	answer := openapi.MailDelivered{Outcome: openapi.MailDeliveredOutcome(result.Outcome)}
	if !result.EntryID.IsZero() {
		answer.EntryId = uuidPointer(result.EntryID)
	}
	if !result.ItemID.IsZero() {
		answer.ItemId = uuidPointer(result.ItemID)
	}
	if !result.CommentID.IsZero() {
		answer.CommentId = uuidPointer(result.CommentID)
	}
	writeJSON(w, r, http.StatusAccepted, answer)
}

// mailAddressResponse maps the address document.
func mailAddressResponse(out usecase.Output) openapi.MailAddress {
	available, _ := out["available"].(bool)
	answer := openapi.MailAddress{Available: available}
	if !available {
		return answer
	}
	if text := out.String("address"); text != "" {
		answer.Address = &text
	}
	if kind := out.String("target_kind"); kind != "" {
		value := openapi.MailAddressTargetKind(kind)
		answer.TargetKind = &value
	}
	if id := out.String("target_id"); id != "" {
		value := uuidValue(id)
		answer.TargetId = &value
	}
	if policy := out.String("sender_policy"); policy != "" {
		value := openapi.MailAddressSenderPolicy(policy)
		answer.SenderPolicy = &value
	}
	if id := out.String("opened_by"); id != "" {
		value := uuidValue(id)
		answer.OpenedBy = &value
	}
	answer.CreatedAt = timePointer(out["created_at"])
	return answer
}

// mimeFields reads the named fields of a multipart form and nothing else: Mailgun's shape, where
// the raw message is `body-mime`. Walked part by part rather than parsed whole, so no file part
// lands on disk; the request middleware has already bounded the body.
func mimeFields(r *http.Request, names ...string) (map[string]string, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	found := map[string]string{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return found, nil
		}
		if err != nil {
			return nil, err
		}
		if wanted[part.FormName()] {
			value, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			found[part.FormName()] = string(value)
		}
		_ = part.Close()
	}
}
