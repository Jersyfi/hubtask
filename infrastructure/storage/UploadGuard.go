// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package storage holds the outbound adapters behind core/port/storage, and the upload guard in
// front of them: the component the upload matrix tests (SG-12, security.md T-11/T-17).
//
// The guard is here rather than in the domain because sniffing is: the signature table lives in
// net/http, which the domain may not import. The *policy* over the sniffed answer - what may
// render inline, which claims are lies - is the domain's (core/domain/model/media), so the split
// is mechanics here, judgement there.
package storage

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/storage"
)

// sniffBytes is what content sniffing reads: the WHATWG algorithm behind
// http.DetectContentType never looks past 512 bytes.
const sniffBytes = 512

// UploadGuard is the port's Guard: the judgement, as something a use case can hold without
// importing this package (core/port/storage).
//
// Empty on purpose. The guard has no state and no configuration - the limit travels with the call,
// because it is the installation's and the application layer is what knows it.
type UploadGuard struct{}

func NewUploadGuard() UploadGuard { return UploadGuard{} }

var _ port.Guard = UploadGuard{}

func (UploadGuard) Inspect(content io.Reader, claimedType string, limit int64) (port.Inspection, error) {
	return Inspect(content, claimedType, limit)
}

// Inspect judges one upload before a byte of it reaches a store.
//
// It reads at most 512 bytes to sniff, asks the domain to reconcile the claim, and hands back a
// stream that enforces the size limit while it is consumed. The limit refusal happens at the
// boundary byte: an upload larger than the limit costs the bytes already streamed and nothing
// held in memory - never an allocation of the object (T-17,
// observability-reliability.md §6).
func Inspect(content io.Reader, claimedType string, limit int64) (port.Inspection, error) {
	head := make([]byte, sniffBytes)
	read, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return port.Inspection{}, shared.ErrValidation.
			WithDetail("media.content_unreadable").
			WithFields(shared.FieldError{Path: "/content", Code: "media.content_unreadable"})
	}
	head = head[:read]
	if read == 0 {
		return port.Inspection{}, shared.ErrValidation.
			WithDetail("media.content_required").
			WithFields(shared.FieldError{Path: "/content", Code: "media.content_required"})
	}
	if limit > 0 && int64(read) > limit {
		return port.Inspection{}, media.TooLarge(limit)
	}

	stored, err := media.AcceptClaim(claimedType, http.DetectContentType(head))
	if err != nil {
		return port.Inspection{}, err
	}

	rest := io.Reader(content)
	if limit > 0 {
		// The head already spent part of the budget. One byte past the remainder is the
		// refusal, not a truncation: a bounded reader that silently stopped would store a
		// file the sender did not send.
		rest = &boundedReader{source: content, limit: limit, remaining: limit - int64(read)}
	}
	return port.Inspection{
		ContentType: stored,
		Content:     io.MultiReader(bytes.NewReader(head), rest),
	}, nil
}

// boundedReader refuses at the boundary byte of its budget.
type boundedReader struct {
	source    io.Reader
	limit     int64
	remaining int64
	exhausted bool
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if b.exhausted {
		return 0, media.TooLarge(b.limit)
	}
	if int64(len(p)) > b.remaining+1 {
		// One byte more than the budget, so the boundary is observed rather than inferred: a
		// read that came back full at exactly the budget cannot say whether the stream ended.
		p = p[:b.remaining+1]
	}
	read, err := b.source.Read(p)
	if int64(read) > b.remaining {
		b.exhausted = true
		return 0, media.TooLarge(b.limit)
	}
	b.remaining -= int64(read)
	return read, err
}
