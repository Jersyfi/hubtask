// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func TestAMediaTokenOpensExactlyWhatItWasMintedFor(t *testing.T) {
	issuer := NewMediaTokenIssuer(secret.New("an installation secret of decent length"))
	mediaID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	otherID := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)

	token := issuer.Issue(MediaTokenUpload, mediaID, expires)

	if err := issuer.Validate(token, MediaTokenUpload, mediaID, now); err != nil {
		t.Fatalf("the minted token was refused: %v", err)
	}
	if err := issuer.Validate(token, MediaTokenDownload, mediaID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("an upload token opened a download: %v", err)
	}
	if err := issuer.Validate(token, MediaTokenUpload, otherID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("the token opened another object: %v", err)
	}
	if err := issuer.Validate(token, MediaTokenUpload, mediaID, expires.Add(time.Second)); shared.AsError(err).DetailCode != "media.upload_expired" {
		t.Errorf("an expired token was answered with %v", err)
	}
	if err := issuer.Validate(token+"x", MediaTokenUpload, mediaID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("a mangled token was answered with %v", err)
	}

	stranger := NewMediaTokenIssuer(secret.New("a different installation secret entirely"))
	if err := stranger.Validate(token, MediaTokenUpload, mediaID, now); shared.AsError(err).DetailCode != "media.token_invalid" {
		t.Errorf("another installation's key accepted the token: %v", err)
	}
}
