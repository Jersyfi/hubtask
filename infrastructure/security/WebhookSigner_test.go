// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	signedAt = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	body     = []byte(`{"specversion":"1.0","type":"de.hubtask.work.item.created.v1"}`)
	signing  = secret.New("a-signing-secret-of-a-webhook-subscription")
)

// The formula automation.md §3.1 fixes, asserted as a shape rather than as a golden string: the
// header is two named parts, the timestamp is seconds, and the signature is hex.
func TestTheSignatureIsTheDocumentedFormula(t *testing.T) {
	header := NewWebhookSigner().Sign(signing, signedAt, body)

	if !strings.HasPrefix(header, "t=") || !strings.Contains(header, ",v1=") {
		t.Fatalf("header = %q, want t=<ts>,v1=<hmac>", header)
	}
	if got := NewWebhookSigner().SignedAt(header); !got.Equal(signedAt) {
		t.Errorf("the timestamp reads back as %v, want %v", got, signedAt)
	}
	// 32 bytes of SHA-256 as hex.
	_, signature, _ := parseSignature(header)
	if len(signature) != 32 {
		t.Errorf("the signature is %d bytes, want 32", len(signature))
	}
}

func TestASignatureVerifiesAgainstItsOwnBodyAndNoOther(t *testing.T) {
	signer := NewWebhookSigner()
	header := signer.Sign(signing, signedAt, body)

	if !signer.Verify(header, body, signing) {
		t.Error("a signature does not verify against the body it was made for")
	}

	// A tampered body does not verify - which is the whole point of signing one.
	tampered := append([]byte{}, body...)
	tampered[10] ^= 0x20
	if signer.Verify(header, tampered, signing) {
		t.Error("a tampered body verified")
	}

	if signer.Verify(header, body, secret.New("a different secret")) {
		t.Error("a signature verified under the wrong secret")
	}
}

// The timestamp is inside the signed string, so a captured delivery cannot be replayed later with
// its own timestamp rewritten.
func TestRewritingTheTimestampInvalidatesTheSignature(t *testing.T) {
	signer := NewWebhookSigner()
	header := signer.Sign(signing, signedAt, body)

	stamp, signature, _ := parseSignature(header)
	rewritten := "t=" + "9999999999" + ",v1=" + strings.TrimPrefix(header, "t="+stamp+",v1=")

	if signer.Verify(rewritten, body, signing) {
		t.Error("a delivery replayed with a rewritten timestamp verified")
	}
	if len(signature) == 0 {
		t.Fatal("the fixture produced no signature")
	}
}

// A rotation leaves two secrets valid for its grace, and a subscriber that has deployed neither
// yet must not be cut off mid-deployment.
func TestEitherSecretOfARotationVerifies(t *testing.T) {
	signer := NewWebhookSigner()
	previous := secret.New("the secret before the rotation")

	old := signer.Sign(previous, signedAt, body)
	current := signer.Sign(signing, signedAt, body)

	if !signer.Verify(old, body, signing, previous) {
		t.Error("a delivery signed with the previous secret did not verify during the grace")
	}
	if !signer.Verify(current, body, signing, previous) {
		t.Error("a delivery signed with the current secret did not verify")
	}
	// And after the grace, when only the current secret is offered, the old one is refused.
	if signer.Verify(old, body, signing) {
		t.Error("the previous secret still verified after its grace")
	}
}

// The version prefix is what lets a second scheme land beside v1 rather than replacing it, so a
// subscriber that verifies only v1 keeps working through the change that introduces v2.
func TestAHeaderIsReadByNameRatherThanByPosition(t *testing.T) {
	signer := NewWebhookSigner()
	header := signer.Sign(signing, signedAt, body)
	stamp, _, _ := parseSignature(header)
	v1 := strings.TrimPrefix(header, "t="+stamp+",")

	for _, reordered := range []string{
		v1 + ",t=" + stamp,
		"t=" + stamp + ",v2=notyet," + v1,
		" t=" + stamp + " , " + v1 + " ",
	} {
		if !signer.Verify(reordered, body, signing) {
			t.Errorf("a header this system should read was refused: %q", reordered)
		}
	}

	for _, nonsense := range []string{"", "t=1", "v1=abc", "t=1,v1=nothex"} {
		if signer.Verify(nonsense, body, signing) {
			t.Errorf("a header that is not ours verified: %q", nonsense)
		}
	}
}
