// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	usecase "github.com/Jersyfi/hubtask/core/application/service/idempotency"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// IdempotencyKeyHeader is the client's handle on a repeated write (api-guidelines.md §5).
const IdempotencyKeyHeader = "Idempotency-Key"

// ReplayedHeader tells a client that it is looking at a stored answer rather than a fresh one.
// Not in the contract as a promise, and deliberately so: a client must behave identically either
// way. It exists for the person debugging why a request "did nothing".
const ReplayedHeader = "Idempotent-Replayed"

// maxStoredResponseBytes bounds what is kept for a replay. A response larger than this is
// answered normally and simply not stored - the alternative is a jsonb column growing without a
// bound anybody chose.
const maxStoredResponseBytes = 256 << 10

// IdempotencyGuard is the slice of the guard use case this middleware needs.
type IdempotencyGuard interface {
	Begin(context.Context, appshared.ActorContext, repository.Key, []byte) (usecase.Attempt, error)
	Complete(context.Context, appshared.ActorContext, repository.Key, int, []byte) error
}

// Idempotent executes a keyed write once and replays its answer for every repeat.
//
// Only POST. The other write methods are idempotent by their own definition - PUT and DELETE
// repeat harmlessly, and PATCH is guarded by If-Match instead (api-guidelines.md §5).
type Idempotent struct {
	Next  http.Handler
	Guard IdempotencyGuard
	// Routes resolves the route template, which is the endpoint half of the key. The template
	// rather than the path: two items are two requests, but the same key sent to two different
	// operations must not share a record.
	Routes Router
}

func (i Idempotent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get(IdempotencyKeyHeader)
	if r.Method != http.MethodPost || key == "" {
		i.Next.ServeHTTP(w, r)
		return
	}

	requestID := correlation.RequestIDFrom(r.Context())
	if !isUUID(key) {
		// The contract types the header as a UUID. Accepting anything else would let a client
		// choose a key another client can guess, and a guessable key is a way to read somebody
		// else's answer within their own tenant.
		WriteProblem(w, shared.ErrValidation.WithDetail("idempotency.key_malformed"), requestID)
		return
	}

	actor, ok := appshared.ActorFrom(r.Context())
	if !ok || !actor.IsAuthenticated() {
		// Unreachable behind the authentication middleware, and a fail-closed guard rather than
		// an assumption: a record without a tenant has nowhere to live.
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		// The body limit is enforced one middleware out; reaching here means the client stopped
		// sending, which is a malformed request rather than a server fault.
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("idempotency.body_unreadable"), requestID)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	_, endpoint := i.Routes.Handler(r)
	if endpoint == "" {
		// The route matched nothing, so there is no operation to make idempotent. Reserving a key
		// against an empty endpoint would leave a record for a request that never ran, and the
		// client's next attempt at a real route would collide with it. The router answers the 404.
		i.Next.ServeHTTP(w, r)
		return
	}
	record := repository.Key{Key: key, Endpoint: endpoint}

	attempt, err := i.Guard.Begin(r.Context(), actor, record, requestHash(r.Method, endpoint, body))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if attempt.IsReplay() {
		writeReplay(w, *attempt.Replay)
		return
	}

	captured := &capturedResponse{ResponseWriter: w, limit: maxStoredResponseBytes}
	i.Next.ServeHTTP(captured, r)

	// Stored after the answer has gone out, and with a context that outlives the request: a
	// client that hung up must not leave a reservation nobody ever completes, because the next
	// repeat would then be refused as "in progress" for as long as the record lives.
	ctx := context.WithoutCancel(r.Context())
	if err := i.Guard.Complete(ctx, actor, record, captured.statusCode(), captured.stored()); err != nil {
		// The client already has its answer. Failing now would report an error for an operation
		// that succeeded, which is worse than a repeat running twice.
		slog.WarnContext(ctx, "storing the idempotent answer failed",
			slog.String("error", err.Error()))
	}
}

func writeReplay(w http.ResponseWriter, record repository.Record) {
	header := w.Header()
	header.Set(ReplayedHeader, "true")
	header.Set("Cache-Control", "no-store")
	if len(record.Body) > 0 {
		header.Set("Content-Type", "application/json")
	}
	w.WriteHeader(record.Status)
	if len(record.Body) > 0 {
		_, _ = w.Write(record.Body)
	}
}

// requestHash is what makes "the same request" a decidable question. The method and the route
// template travel with the body, so the same body sent to a different operation under the same
// key is recognised as a different request rather than replayed.
func requestHash(method, endpoint string, body []byte) []byte {
	digest := sha256.New()
	digest.Write([]byte(method))
	digest.Write([]byte{0})
	digest.Write([]byte(endpoint))
	digest.Write([]byte{0})
	digest.Write(body)
	return digest.Sum(nil)
}

// capturedResponse passes the answer through to the client and keeps a copy for the store, up to
// a limit. Over the limit it keeps nothing: a partial body stored as a whole one would be
// replayed as a truncated answer, which is worse than not replaying at all.
type capturedResponse struct {
	http.ResponseWriter
	status   int
	written  bool
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (c *capturedResponse) WriteHeader(status int) {
	if c.written {
		return
	}
	c.status, c.written = status, true
	c.ResponseWriter.WriteHeader(status)
}

func (c *capturedResponse) Write(b []byte) (int, error) {
	if !c.written {
		c.WriteHeader(http.StatusOK)
	}
	if !c.overflow {
		if c.buffer.Len()+len(b) > c.limit {
			c.overflow = true
			c.buffer.Reset()
		} else {
			c.buffer.Write(b)
		}
	}
	return c.ResponseWriter.Write(b)
}

func (c *capturedResponse) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *capturedResponse) statusCode() int {
	if !c.written {
		return http.StatusOK
	}
	return c.status
}

func (c *capturedResponse) stored() []byte {
	if c.overflow {
		return nil
	}
	return c.buffer.Bytes()
}

// isUUID accepts the canonical 8-4-4-4-12 form in either case. Looser than the identifier parser
// of the domain, which insists on lower case: this value is a client's own handle and is never
// an identifier of anything stored.
func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isDigit := c >= '0' && c <= '9'
			isHex := (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isDigit && !isHex {
				return false
			}
		}
	}
	return true
}
