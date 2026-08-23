// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package media is the Media context's domain: what an uploaded object is, and how it may be
// served (arc42 §5.2). C-05 brings the delivery policy the upload matrix tests (SG-12); the
// MediaObject aggregate arrives with C-06, which owns the upload flow.
package media

import (
	"mime"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Disposition is how an object may reach a browser (T-11).
//
// Two answers, and the safe one is the default. Inline is earned by being on a short allowlist
// of types a browser renders without executing anything; everything else - SVG, HTML, PDFs,
// polyglot files, and every type nobody recognised - is served as a download, from an origin
// that is not the application's, under `Content-Disposition: attachment` and a
// `Content-Security-Policy: sandbox` (security.md §9). Inert is not a judgement about the file;
// it is the refusal to let a browser make one.
type Disposition string

const (
	// DispositionInline may render in the page: the type is an image format whose decoding
	// executes nothing.
	DispositionInline Disposition = "INLINE"
	// DispositionAttachment is served as a download only.
	DispositionAttachment Disposition = "ATTACHMENT"
)

// inlineTypes is the allowlist. Short on purpose: a type is added here when somebody shows its
// rendering path executes nothing, not removed when somebody shows it does.
var inlineTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// DeliveryFor answers how an object of this content type may be served. Called with the stored
// type, which is the sniffed one - never the client's claim (T-11).
func DeliveryFor(contentType string) Disposition {
	if inlineTypes[essence(contentType)] {
		return DispositionInline
	}
	return DispositionAttachment
}

// AcceptClaim reconciles what the client said with what the bytes are, and returns the type to
// store.
//
// The sniffed type wins (T-11): it is what the bytes will do in a browser, and the claim is what
// the sender hoped. The claim still matters twice. A claim that contradicts the sniff on an
// inline-capable type is refused rather than corrected - claiming image/png for HTML is the
// smuggling the matrix exists to catch, and silently storing "text/html" would accept a file the
// sender lied about. And a claim may *sharpen* a sniff that could not decide: sniffing cannot
// name SVG (it reads as XML) or a format it has never heard of, so a non-inline claim over a
// generic sniff is stored as claimed - it changes nothing about delivery, which stays a download
// for everything off the allowlist.
func AcceptClaim(claimed, sniffed string) (string, error) {
	sniffedEssence := essence(sniffed)
	if claimed == "" {
		return sniffedEssence, nil
	}
	claimedEssence := essence(claimed)
	if claimedEssence == "" {
		return "", typeMismatch(claimed, sniffed)
	}

	if claimedEssence == sniffedEssence {
		return sniffedEssence, nil
	}

	// The sniff was generic: the bytes matched no signature, or only "some XML", "some text".
	// A sharper claim is kept when it cannot buy the file a rendering path - a claim on the
	// inline allowlist over bytes that do not sniff as that type is exactly the lie T-11 names.
	if genericTypes[sniffedEssence] && !inlineTypes[claimedEssence] {
		return claimedEssence, nil
	}

	return "", typeMismatch(claimed, sniffed)
}

// genericTypes is what sniffing answers when it recognised nothing specific.
var genericTypes = map[string]bool{
	"application/octet-stream": true,
	"text/plain":               true,
	"text/xml":                 true,
}

func typeMismatch(claimed, sniffed string) error {
	return shared.ErrValidation.
		WithDetail("media.type_mismatch").
		WithParams(map[string]string{"claimed": essence(claimed), "sniffed": essence(sniffed)}).
		WithFields(shared.FieldError{Path: "/content_type", Code: "media.type_mismatch"})
}

// essence is the type without its parameters, lower-cased: "text/HTML; charset=utf-8" and
// "text/html" are one type to a browser, so they are one type here.
func essence(contentType string) string {
	parsed, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed)
}

// TooLarge is the refusal of an upload past the limit (T-17). Declared here, beside the policy
// it belongs to; the guard that enforces it streams, so the refusal happens at the boundary
// byte, never after buffering the object.
func TooLarge(limit int64) error {
	return shared.ErrValidation.
		WithDetail("media.too_large").
		WithParams(map[string]string{"limit_bytes": strconv.FormatInt(limit, 10)}).
		WithFields(shared.FieldError{Path: "/content", Code: "media.too_large"})
}
