// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Bounded caps what one request may cost: how much body it may send, and how long it may run.
//
// Both are the server's decision, not the client's. A handler that forgets its own deadline still
// gets one here, which is what rule 7 of CLAUDE.md asks for - no call without a deadline
// (ADR-0016) - and a body limit is the cheapest defence against T-17 there is.
type Bounded struct {
	Next http.Handler
	// MaxBodyBytes is the global limit for everything the contract carries as JSON (security.md §9).
	MaxBodyBytes int64
	// MaxUploadBytes is the limit for the one route the contract declares as a byte stream: the
	// content route of a local-storage installation, which stands in for the bucket a presigned
	// upload would have gone to (C-06). Bounding it by MaxBodyBytes would make an attachment on
	// such an installation impossible; bounding everything else by this one would hand every JSON
	// endpoint a sixty-four megabyte budget for a body that is never that big.
	MaxUploadBytes int64
	// Timeout is the deadline every handler inherits through the request context.
	Timeout time.Duration
}

// streamSuffix is the one route with no end of its own: a stream is held open until the client
// leaves or the process drains, which is the opposite of what a request timeout is for (C-10).
const streamSuffix = "/stream"

// deadlineFor is how long this request may run.
//
// Every request gets the configured deadline, which is what rule 7 asks for - and exactly one does
// not, because for it the deadline would be a bug rather than a bound. A stream cut off after the
// request timeout would look to a client like a server that drops connections on a timer, and the
// client would reconnect on that timer forever.
//
// What bounds a stream instead is everything the handler already selects on: the client's own
// context, the shutdown, and a heartbeat that fails to write. Recognised by method and shape for
// the reason limitFor recognises the byte route - the alternative is a list in the composition root
// that drifts from the contract.
func (b Bounded) deadlineFor(r *http.Request) time.Duration {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, streamSuffix) {
		return 0
	}
	return b.Timeout
}

// limitFor is which of the two bounds applies. It recognises the byte route rather than being told
// about it, because the alternative is a list in the composition root that drifts from the
// contract - and it recognises it by method and shape, so a GET on the same path is bounded as
// everything else is.
func (b Bounded) limitFor(r *http.Request) int64 {
	if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, mediaContentSuffix) {
		return b.MaxUploadBytes
	}
	return b.MaxBodyBytes
}

func (b Bounded) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A declared length over the limit is refused before a byte is read. Without this check the
	// answer would come only after the whole body had been transferred - which is the cost the
	// limit exists to avoid.
	limit := b.limitFor(r)
	if limit > 0 && r.ContentLength > limit {
		WriteTooLarge(w, limit, correlation.RequestIDFrom(r.Context()))
		return
	}
	if limit > 0 && r.Body != nil {
		// The undeclared case: a chunked body has no Content-Length, and this is what stops it
		// once it grows past the limit. The read then fails, and the handler's decoder turns that
		// into malformed_request.
		r.Body = http.MaxBytesReader(w, r.Body, limit)
	}

	timeout := b.deadlineFor(r)
	if timeout <= 0 {
		b.Next.ServeHTTP(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	b.Next.ServeHTTP(w, r.WithContext(ctx))
}

// Localised resolves the language and the time zone of the request and puts an actor into the
// context carrying them.
//
// It runs before authentication, because an unauthenticated answer needs a language too - a 401
// is rendered by the client from a code and its parameters, and the client needs to know which
// language it was asked for. Authentication then replaces the anonymous actor with the real one,
// keeping the account's and the tenant's preference where they exist (i18n-l10n.md §2).
type Localised struct {
	Next   http.Handler
	Locale env.LocaleConfig
}

func (l Localised) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Empty when the client stated no usable preference. The distinction matters one middleware
	// later: an account's own language wins over the installation default, but not over a
	// language the client asked for.
	requested := preferredLocale(r.Header.Get("Accept-Language"), "")

	actor := appshared.Anonymous(
		firstNonEmpty(requested, l.Locale.DefaultLocale), l.Locale.DefaultTimeZone)

	ctx := context.WithValue(r.Context(), requestedLocaleKey, requested)
	ctx = appshared.ContextWithActor(ctx, actor)
	l.Next.ServeHTTP(w, r.WithContext(ctx))
}

type contextKey struct{ name string }

// requestedLocaleKey carries what the client asked for, as opposed to what was resolved. It stays
// inside this package: the resolved value lives on the actor, and that is what the application
// layer reads.
var requestedLocaleKey = contextKey{"rest.requested_locale"}

func requestedLocaleFrom(ctx context.Context) string {
	locale, _ := ctx.Value(requestedLocaleKey).(string)
	return locale
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// maxAcceptLanguageLength bounds what is parsed. The header comes from outside, it ends up in a
// context and in a response, and a client with an opinion about 400 languages is not one worth
// answering carefully.
const maxAcceptLanguageLength = 256

// preferredLocale picks the highest-weighted acceptable tag from Accept-Language (RFC 9110 §12.5.4).
//
// It resolves only the request end of the chain; the account and the tenant are read once
// authentication has produced them. A tag is checked for shape rather than against a catalogue:
// the catalogue is the client's business, since the backend emits codes and never sentences
// (ADR-0011). An unusable header falls back to the installation default rather than failing - a
// wrong language is a nuisance, a rejected request is an outage.
func preferredLocale(header, fallback string) string {
	if header == "" || len(header) > maxAcceptLanguageLength {
		return fallback
	}

	best, bestQuality := fallback, 0.0
	for _, entry := range strings.Split(header, ",") {
		tag, parameters, _ := strings.Cut(strings.TrimSpace(entry), ";")
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "*" || !isLanguageTag(tag) {
			continue
		}

		quality := 1.0
		if raw, found := strings.CutPrefix(strings.TrimSpace(parameters), "q="); found {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil || parsed < 0 || parsed > 1 {
				continue
			}
			quality = parsed
		}
		// q=0 means "not acceptable", and strictly greater keeps the first of equal weights,
		// which is the order the client listed them in.
		if quality > bestQuality {
			best, bestQuality = tag, quality
		}
	}
	return best
}

// maxLanguageTagLength is generous for a BCP 47 tag (de-AT, pt-BR, zh-Hans-CN) and still far
// short of anything worth carrying around.
const maxLanguageTagLength = 35

// isLanguageTag checks the shape of a BCP 47 tag: subtags of letters or digits, separated by
// hyphens. Enough to keep a header value from becoming a log injection or a reflected payload,
// which is all this layer owes (security.md §7).
func isLanguageTag(tag string) bool {
	if len(tag) < 2 || len(tag) > maxLanguageTagLength {
		return false
	}
	for _, subtag := range strings.Split(tag, "-") {
		if subtag == "" {
			return false
		}
		for _, c := range subtag {
			isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
			isDigit := c >= '0' && c <= '9'
			if !isLetter && !isDigit {
				return false
			}
		}
	}
	return true
}
