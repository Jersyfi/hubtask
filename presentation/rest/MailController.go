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

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The mail intake (ADR-0094). The controller maps documents; who may see or change the settings,
// and where a delivery goes, are decided inwards of here.

// AddressedMailDeliverer serves the receiving domain's inbound route. Not a catalogue entry: it
// answers the installation's inbound key, which no person holds.
type AddressedMailDeliverer interface {
	Authorized(header string) bool
	Deliver(ctx context.Context, recipient string, raw []byte) (jumbleservice.MailDeliveryResult, error)
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
	if result.Reason != "" {
		reason := openapi.MailReason(result.Reason)
		answer.Reason = &reason
	}
	if result.Duplicate {
		answer.Duplicate = &result.Duplicate
	}
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
