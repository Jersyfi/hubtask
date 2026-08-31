// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package storage is the port for object storage: the bytes a person attaches to their work
// (arc42 §5.2 Media, §8.4).
//
// The content travels as a stream, and that is the point of the shape. The httpclient port
// argues the opposite - everything Hubtask sends outwards is a small, already-rendered payload -
// and media is exactly the payload that breaks the premise: an upload is bounded by
// HUBTASK_MAX_UPLOAD_BYTES (64 MiB by default), GOMEMLIMIT is set, and an object read into
// memory is an OOM kill waiting for a large file, which observability-reliability.md §6 calls an
// architecture defect rather than an operations problem (T-17).
//
// What the port does not know: where the bytes live (a directory or a bucket - the adapter's
// business), whether the content is safe to serve (the upload guard's business, decided before a
// byte reaches this port), and who may touch the object (the application layer's business,
// ADR-0005). A key is an opaque name the application mints - a UUID-shaped path, never anything
// a client typed - and an adapter still refuses one that tries to walk out of its namespace,
// because defence in depth is cheaper than certainty about every future caller.
package storage

import (
	"context"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
)

// ObjectStore reads and writes objects. Implementations: LocalStorage (a directory, the
// self-hosting default) and S3Storage (S3 and S3-compatible services - MinIO, Garage).
//
// The error contract is the shared one: a missing object is ErrNotFound with the detail
// `media.object_missing` and the key as its parameter, an unreachable backend is ErrUnavailable
// with a `dependency.` detail - never a raw driver message (T-18). The detail is on the adapter's
// answer rather than only on the caller's translation of it, so that a store answering "no bytes
// here" is distinguishable from a record that was never there, wherever the error is read. No call
// runs inside a database transaction (observability-reliability.md §8): storage is an external
// dependency, and a transaction waiting on a bucket holds a connection for as long as somebody
// else's server feels like taking.
type ObjectStore interface {
	// Put writes one object whole. The size is declared rather than discovered, because the
	// adapter needs it before the first byte (Content-Length for S3, preallocation for a disk) -
	// and the caller has it: the upload guard counted the stream against the limit already.
	Put(ctx context.Context, upload Upload) error

	// Get returns one object for streaming. The caller owns the content and closes it; an
	// adapter must not buffer the object to serve it (T-17).
	Get(ctx context.Context, key string) (Object, error)

	// Delete removes one object. Removing what is not there succeeds - deletion is the state
	// the caller asked for, and the reconciliation job that calls this retries (C-06,
	// data-protection.md §5).
	Delete(ctx context.Context, key string) error
}

// Inspection is a judged upload: the type to store, and the content re-assembled for exactly one
// consumption.
type Inspection struct {
	// ContentType is the type to store: the sniffed one, sharpened by the claim only where the
	// domain's policy allows (media.AcceptClaim). Never the claim on its own (T-11).
	ContentType string
	// Content replays the sniffed head and continues with the rest, refusing at the boundary byte
	// of the limit. It has not been buffered beyond that head and must be consumed once, as a
	// stream (T-17).
	Content io.Reader
}

// Guard judges bytes before any store sees them: the upload matrix's subject (SG-12, T-11, T-17).
//
// A port rather than a call into the adapter, because a use case may not import one - and the
// split it draws is real: sniffing is mechanics, and the signature table lives in net/http, which
// the core may not import. The policy over the sniffed answer is the domain's
// (core/domain/model/media).
type Guard interface {
	// Inspect reads at most the sniffing head, reconciles the claim against what the bytes are,
	// and hands back a stream that enforces the limit while it is consumed. A limit of zero is no
	// limit.
	Inspect(content io.Reader, claimedType string, limit int64) (Inspection, error)
}

// Transfer is one side of the byte movement, as the contract's MediaTransfer carries it: a URL
// that is itself the capability, the method to use on it, and the moment it stops working.
type Transfer struct {
	URL       string
	Method    string
	ExpiresAt time.Time
}

// TransferIssuer mints the URLs the bytes travel through, so the server never carries them on an
// object-storage installation (arc42 §8.4) - and stands in for that with its own token-protected
// routes on a local one.
//
// The whole record travels rather than its parts, because the two implementations address by
// different halves of it: a presigned URL names the storage key and the file name, the local
// content route names the object and the tenant its bytes live under and resolves the key itself.
// Passing four parameters instead would be passing the union of both, with each implementation
// ignoring the half it does not use.
type TransferIssuer interface {
	// IssueUpload mints where the staged object's bytes go.
	IssueUpload(object media.Object, expiresAt time.Time) (Transfer, error)

	// IssueDownload mints where the object's bytes come from, served as a download: the
	// disposition - attachment, with the file's name when one is known - is part of what is
	// signed, so a holder cannot strip it (T-11).
	IssueDownload(object media.Object, expiresAt time.Time) (Transfer, error)
}

// Upload is one object on its way in.
type Upload struct {
	// Key is where the object lives, minted by the application: opaque, slash-separated, never
	// user text.
	Key string
	// Content is the bytes, already judged by the upload guard: sniffed, size-checked, and
	// exactly Size long.
	Content io.Reader
	// Size is the exact length of Content in bytes.
	Size int64
	// ContentType is the sniffed type - never the client's claim (T-11).
	ContentType string
}

// Object is one object on its way out.
type Object struct {
	// Content streams the bytes. The caller closes it.
	Content io.ReadCloser
	Size    int64
	// ContentType is the type as stored, which is the sniffed one.
	ContentType string
}
