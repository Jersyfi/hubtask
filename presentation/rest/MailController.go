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

// The mail intake (ADR-0094). The controller maps documents; who may see or change the settings,
// and where a delivery goes, are decided inwards of here.

const (
	showMailSettingsUseCase    = "ShowMailSettings"
	saveMailSettingsUseCase    = "SaveMailSettings"
	listMailReceiptsUseCase    = "ListMailReceipts"
	setCollectionHandleUseCase = "SetCollectionHandle"
	listMailboxesUseCase       = "ListMailboxes"
	connectMailboxUseCase      = "ConnectMailbox"
	checkMailboxUseCase        = "CheckMailbox"
	removeMailboxUseCase       = "RemoveMailbox"
	authorizeMailboxUseCase    = "AuthorizeMailbox"
	completeMailboxUseCase     = "CompleteMailboxAuthorization"
	commentJumbleEntryUseCase  = "CommentJumbleEntry"
)

// AddressedMailDeliverer serves the receiving domain's inbound route. Not a catalogue entry: it
// answers the installation's inbound key, which no person holds.
type AddressedMailDeliverer interface {
	Authorized(header string) bool
	Deliver(ctx context.Context, recipient string, raw []byte) (jumbleservice.MailDeliveryResult, error)
}

// ShowMailSettings answers GET /mail/settings.
func (c *RestController) ShowMailSettings(w http.ResponseWriter, r *http.Request) {
	c.mailDocument(w, r, showMailSettingsUseCase, usecase.Input{}, &openapi.MailSettings{})
}

// SaveMailSettings answers PUT /mail/settings.
func (c *RestController) SaveMailSettings(w http.ResponseWriter, r *http.Request, _ openapi.SaveMailSettingsParams) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	c.mailDocument(w, r, saveMailSettingsUseCase, usecase.Input(body), &openapi.MailSettings{})
}

// ListMailReceipts answers GET /mail/receipts.
func (c *RestController) ListMailReceipts(w http.ResponseWriter, r *http.Request) {
	c.mailDocument(w, r, listMailReceiptsUseCase, usecase.Input{}, &openapi.MailReceiptList{})
}

// SetCollectionHandle answers PUT /collections/{collection_id}/handle.
func (c *RestController) SetCollectionHandle(
	w http.ResponseWriter, r *http.Request, collectionID openapi_types.UUID, _ openapi.SetCollectionHandleParams,
) {
	var body openapi.CollectionHandle
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	input := usecase.Input{"collection_id": collectionID.String(), "handle": body.Handle}
	c.mailDocument(w, r, setCollectionHandleUseCase, input, &openapi.CollectionHandle{})
}

// ListMailboxes answers GET /mail/mailboxes.
func (c *RestController) ListMailboxes(w http.ResponseWriter, r *http.Request) {
	c.mailDocument(w, r, listMailboxesUseCase, usecase.Input{}, &openapi.MailboxList{})
}

// ConnectMailbox answers POST /mail/mailboxes.
func (c *RestController) ConnectMailbox(w http.ResponseWriter, r *http.Request, _ openapi.ConnectMailboxParams) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	if port, ok := body["port"].(float64); ok {
		body["port"] = int(port)
	}
	c.mailDocumentWith(w, r, http.StatusCreated, connectMailboxUseCase, usecase.Input(body), &openapi.Mailbox{})
}

// AuthorizeMailbox answers POST /mail/mailboxes:authorize.
func (c *RestController) AuthorizeMailbox(w http.ResponseWriter, r *http.Request, _ openapi.AuthorizeMailboxParams) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	if port, ok := body["port"].(float64); ok {
		body["port"] = int(port)
	}
	c.mailDocument(w, r, authorizeMailboxUseCase, usecase.Input(body), &openapi.MailboxAuthorization{})
}

// CompleteMailboxAuthorization answers POST /mail/mailboxes:complete.
func (c *RestController) CompleteMailboxAuthorization(
	w http.ResponseWriter, r *http.Request, _ openapi.CompleteMailboxAuthorizationParams,
) {
	var body openapi.MailboxAuthorizationAnswer
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	input := usecase.Input{"state": body.State}
	if body.Code != nil {
		input["code"] = *body.Code
	}
	if body.Error != nil {
		input["error"] = *body.Error
	}
	c.mailDocumentWith(w, r, http.StatusCreated, completeMailboxUseCase, input, &openapi.Mailbox{})
}

// CheckMailbox answers POST /mail/mailboxes/{mailbox_id}:check.
func (c *RestController) CheckMailbox(
	w http.ResponseWriter, r *http.Request, mailboxID openapi_types.UUID, _ openapi.CheckMailboxParams,
) {
	c.mailDocument(w, r, checkMailboxUseCase, usecase.Input{"mailbox_id": mailboxID.String()}, &openapi.Mailbox{})
}

// RemoveMailbox answers DELETE /mail/mailboxes/{mailbox_id}.
func (c *RestController) RemoveMailbox(w http.ResponseWriter, r *http.Request, mailboxID openapi_types.UUID) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), removeMailboxUseCase, actorOf(r), usecase.Input{"mailbox_id": mailboxID.String()}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mailDocument invokes a use case as the caller and answers its output in the contract's shape,
// mapped through its JSON spelling: the output already uses the contract's words.
func (c *RestController) mailDocument(
	w http.ResponseWriter, r *http.Request, name string, input usecase.Input, target any,
) {
	c.mailDocumentWith(w, r, http.StatusOK, name, input, target)
}

func (c *RestController) mailDocumentWith(
	w http.ResponseWriter, r *http.Request, status int, name string, input usecase.Input, target any,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), input)
	if err == nil {
		err = throughJSON(out, target)
	}
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, status, target)
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
