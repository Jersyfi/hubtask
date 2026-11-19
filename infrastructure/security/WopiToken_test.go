// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/wopi"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A WOPI token opens one file for one person, read or write, until its expiry - and nothing else.
func TestAWopiTokenOpensOneFileForOnePerson(t *testing.T) {
	issuer := NewWopiTokenIssuer(secret.New("an installation secret of decent length"))
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	file := shared.MustParseID("01900000-0000-7000-8000-00000000000f")
	other := shared.MustParseID("01900000-0000-7000-8000-0000000000aa")
	grant := wopi.Grant{
		TenantID:  shared.MustParseID("01900000-0000-7000-8000-000000000001"),
		AccountID: shared.MustParseID("01900000-0000-7000-8000-000000000002"),
		MediaID:   file, Write: true, ExpiresAt: now.Add(2 * time.Hour),
	}
	token := issuer.Issue(grant)

	got, err := issuer.Read(token, file, now)
	if err != nil || got != grant {
		t.Fatalf("read %+v, %v", got, err)
	}
	read := grant
	read.Write = false
	if got, _ := issuer.Read(issuer.Issue(read), file, now); got.Write {
		t.Error("a read token reads as a write token")
	}

	refused := map[string]func() error{
		"another file": func() error { _, err := issuer.Read(token, other, now); return err },
		"expired":      func() error { _, err := issuer.Read(token, file, now.Add(3*time.Hour)); return err },
		"another key": func() error {
			_, err := NewWopiTokenIssuer(secret.New("a different installation secret entirely")).Read(token, file, now)
			return err
		},
		"garbage": func() error { _, err := issuer.Read("not-a-token", file, now); return err },
		"the write bit flipped": func() error {
			raw := []byte(token)
			// The flag sits at a fixed place; any change to the token breaks its tag.
			raw[len(raw)-1] ^= 1
			_, err := issuer.Read(string(raw), file, now)
			return err
		},
		"a media token": func() error {
			media := NewMediaTokenIssuer(secret.New("an installation secret of decent length"))
			_, err := issuer.Read(media.Issue(MediaTokenDownload, grant.TenantID, file, now.Add(time.Hour)), file, now)
			return err
		},
	}
	for name, try := range refused {
		if err := try(); !errors.Is(err, shared.ErrUnauthenticated) || shared.AsError(err).DetailCode != "wopi.token_invalid" {
			t.Errorf("%s: %v", name, err)
		}
	}
}
