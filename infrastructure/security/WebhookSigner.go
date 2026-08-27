// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SignatureHeader is the header a subscriber verifies (automation.md §3.1).
const SignatureHeader = "X-Hubtask-Signature"

// SignatureTolerance is how far a signature's timestamp may be from the receiver's clock before it
// should be refused as a replay. It is documented here rather than enforced here: this side signs,
// and the window is the subscriber's to apply - but the number they should apply is ours to state,
// and stating it in the code that produces the timestamp is the only place it cannot drift from.
const SignatureTolerance = 5 * time.Minute

// WebhookSigner produces the signature line of one delivery.
//
// `t=<unix seconds>,v1=<hex hmac-sha256 of "<t>.<body>">`, which is the shape automation.md §3.1
// fixes and which Stripe made the convention a subscriber's library already knows.
//
// Two properties are worth naming because they are what the shape buys. The timestamp is inside
// the signed string, so a captured delivery cannot be replayed later with its own timestamp
// rewritten - changing `t` invalidates `v1`. And the version prefix means a second scheme can be
// added beside `v1` rather than replacing it, so a subscriber that verifies only `v1` keeps
// working through the change that introduces `v2`.
type WebhookSigner struct{}

func NewWebhookSigner() WebhookSigner { return WebhookSigner{} }

// Sign renders the header value for one body at one moment.
func (WebhookSigner) Sign(signing secret.Secret, at time.Time, body []byte) string {
	stamp := strconv.FormatInt(at.UTC().Unix(), 10)
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac(signing, stamp, body))
}

// Verify reports whether a header value matches a body under one of the secrets given.
//
// Several secrets, because a rotation leaves two valid for its grace, and because this exists for
// the tests and the end-to-end session rather than for the delivery path - this system signs and
// does not receive. It is here so that "the signature verifies against the documented formula" is
// a thing a test can assert with our own code rather than with a second implementation of it,
// which would only prove the two copies agree.
func (WebhookSigner) Verify(header string, body []byte, secrets ...secret.Secret) bool {
	stamp, signature, found := parseSignature(header)
	if !found {
		return false
	}

	for _, signing := range secrets {
		if signing.IsEmpty() {
			continue
		}
		// Constant time: a comparison that returned early would tell an attacker how much of a
		// guess was right, one byte at a time (security.md §8).
		if hmac.Equal(signature, mac(signing, stamp, body)) {
			return true
		}
	}
	return false
}

// SignedAt reads the timestamp out of a header value, so that a receiver can apply the replay
// window. Zero when the header is not one of ours.
func (WebhookSigner) SignedAt(header string) time.Time {
	stamp, _, found := parseSignature(header)
	if !found {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func mac(signing secret.Secret, stamp string, body []byte) []byte {
	writer := hmac.New(sha256.New, []byte(signing.Reveal()))
	writer.Write([]byte(stamp))
	writer.Write([]byte("."))
	writer.Write(body)
	return writer.Sum(nil)
}

// parseSignature reads `t=…,v1=…` and refuses everything else. Order is not assumed: a subscriber
// library that reorders the pair is not wrong, and neither is one that adds a `v2=` beside them.
func parseSignature(header string) (stamp string, signature []byte, ok bool) {
	for _, part := range strings.Split(header, ",") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch name {
		case "t":
			stamp = value
		case "v1":
			decoded, err := hex.DecodeString(value)
			if err != nil {
				return "", nil, false
			}
			signature = decoded
		}
	}
	return stamp, signature, stamp != "" && len(signature) > 0
}
