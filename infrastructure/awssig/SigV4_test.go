// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package awssig_test

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/infrastructure/awssig"
)

// The signing-key derivation against the vector Amazon publishes with the specification. The
// full signature is proved by MinIO in the conformance suite - MinIO validates strictly - and
// this pins the one step a typo would silently break everywhere.
func TestTheSigningKeyMatchesThePublishedVector(t *testing.T) {
	key := awssig.DeriveKey("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "20120215", "us-east-1", "iam")

	want := "f4780e2d9f65fa895f9c67b32ce1baf0b0d8a43505a000a1a9e090d414db404d"
	if got := hex.EncodeToString(key); got != want {
		t.Fatalf("derived %s\nwant    %s", got, want)
	}
}

func TestTheCanonicalFormsFollowTheSpecification(t *testing.T) {
	if got := awssig.URIEncode("a b/c~d._-*"); got != "a%20b%2Fc~d._-%2A" {
		t.Errorf("uriEncode = %q", got)
	}
	if got := awssig.CanonicalURI("/bucket/key with space"); got != "/bucket/key%20with%20space" {
		t.Errorf("canonicalURI = %q", got)
	}
	if got := awssig.CanonicalQuery("b=2&a=1"); got != "a=1&b=2" {
		t.Errorf("canonicalQuery did not sort: %q", got)
	}
}

func TestSigningWritesTheThreeHeaders(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "https://minio.internal:9000/media/key", nil)
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	awssig.Sign(req, "access", "secret", "eu-central-1", "s3", awssig.EmptyPayloadHash, at)

	if got := req.Header.Get("x-amz-date"); got != "20260823T120000Z" {
		t.Errorf("x-amz-date = %q", got)
	}
	if got := req.Header.Get("x-amz-content-sha256"); got != awssig.EmptyPayloadHash {
		t.Errorf("x-amz-content-sha256 = %q", got)
	}
	authorization := req.Header.Get("Authorization")
	for _, part := range []string{
		"AWS4-HMAC-SHA256 Credential=access/20260823/eu-central-1/s3/aws4_request",
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date",
		"Signature=",
	} {
		if !strings.Contains(authorization, part) {
			t.Errorf("the Authorization header misses %q: %s", part, authorization)
		}
	}
}

// The step that is easy to leave out and impossible to debug from the answer: a RawQuery has
// already been percent-encoded, and encoding it again signs %2F as %252F - a signature over a
// string the server will never compute. It shows up as a 403 with no detail, on exactly the
// requests that carry a slash in a parameter, which is every listing under a prefix.
func TestTheCanonicalQueryDoesNotEncodeWhatIsAlreadyEncoded(t *testing.T) {
	cases := map[string]string{
		"prefix=instance%2F":              "prefix=instance%2F",
		"prefix=a%20b":                    "prefix=a%20b",
		"prefix=a+b":                      "prefix=a%20b",
		"list-type=2&prefix=x%2Fy":        "list-type=2&prefix=x%2Fy",
		"uploadId=abc%3Ddef&partNumber=1": "partNumber=1&uploadId=abc%3Ddef",
	}

	for raw, want := range cases {
		if got := awssig.CanonicalQuery(raw); got != want {
			t.Errorf("CanonicalQuery(%q) = %q, want %q", raw, got, want)
		}
	}
}
