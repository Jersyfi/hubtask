// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func TestAMediaTokenOpensExactlyWhatItWasMintedFor(t *testing.T) {
	issuer := NewMediaTokenIssuer(secret.New("an installation secret of decent length"))
	tenantID := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	mediaID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	otherID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)

	token := issuer.Issue(MediaTokenUpload, tenantID, mediaID, expires)

	opened, err := issuer.Validate(token, MediaTokenUpload, mediaID, now)
	if err != nil {
		t.Fatalf("the minted token was refused: %v", err)
	}
	// The tenant is what the content route opens its transaction as, so it has to come back and
	// come back right - there is nothing else on the request that could say it.
	if opened != tenantID {
		t.Errorf("the token opened tenant %q, want the one it was minted in", opened)
	}

	refusals := []struct {
		name string
		err  error
		want string
	}{
		{"an upload token opening a download",
			second(issuer.Validate(token, MediaTokenDownload, mediaID, now)), "media.token_invalid"},
		{"the token opening another object",
			second(issuer.Validate(token, MediaTokenUpload, otherID, now)), "media.token_invalid"},
		{"a mangled token",
			second(issuer.Validate(token+"x", MediaTokenUpload, mediaID, now)), "media.token_invalid"},
		{"an expired token",
			second(issuer.Validate(token, MediaTokenUpload, mediaID, expires.Add(time.Second))),
			"media.upload_expired"},
	}
	for _, refusal := range refusals {
		if got := shared.AsError(refusal.err).DetailCode; got != refusal.want {
			t.Errorf("%s: %q, want %s", refusal.name, got, refusal.want)
		}
	}

	stranger := NewMediaTokenIssuer(secret.New("a different installation secret entirely"))
	if _, err := stranger.Validate(token, MediaTokenUpload, mediaID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("another installation's key accepted the token: %v", err)
	}
}

// The tenant is signed, not merely carried: a token whose tenant was swapped for another one is a
// forgery, and it is refused as one.
func TestTheTenantInAMediaTokenCannotBeSwapped(t *testing.T) {
	issuer := NewMediaTokenIssuer(secret.New("an installation secret of decent length"))
	tenantID := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	otherTenant := shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
	mediaID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	token := issuer.Issue(MediaTokenUpload, tenantID, mediaID, now.Add(5*time.Minute))
	forged := swapTenant(t, token, tenantID, otherTenant)

	if _, err := issuer.Validate(forged, MediaTokenUpload, mediaID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("a token carrying somebody else's tenant was accepted: %v", err)
	}
}

func second(_ shared.ID, err error) error { return err }

// swapTenant rewrites the token's tenant half without touching its tag - which is exactly what
// somebody holding one URL and wanting another tenant's namespace would try.
func swapTenant(t *testing.T, token string, from, to shared.ID) string {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding the token: %v", err)
	}
	head := raw[:mediaTokenTagLength+8]
	if string(raw[mediaTokenTagLength+8:]) != from.String() {
		t.Fatalf("the token does not carry the tenant where it was expected")
	}
	return base64.RawURLEncoding.EncodeToString(append(head, to.String()...))
}
