// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// platformCursorInfo separates this use of the installation secret from every other cursor's, so a
// trigger poll's cursor is never accepted as a position in the installation's events.
const platformCursorInfo = "hubtask/platform-event-cursor/v1"

// errPlatformCursorInvalid is the page cursor's own refusal: the client sent a cursor, and which
// codec read it is the server's business.
var errPlatformCursorInvalid = errCursorInvalid

// PlatformEventCursorCodec turns a position in the installation's events into an opaque cursor and
// back - signed, for the trigger cursor's reason: it decides where a platform resumes.
type PlatformEventCursorCodec struct {
	key []byte
}

// NewPlatformEventCursorCodec keys the codec on the installation secret.
func NewPlatformEventCursorCodec(installationSecret secret.Secret) PlatformEventCursorCodec {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(platformCursorInfo))
	return PlatformEventCursorCodec{key: mac.Sum(nil)}
}

// Encode returns the cursor for a position.
func (c PlatformEventCursorCodec) Encode(position adminrepo.PlatformPosition) string {
	payload := []byte(strconv.FormatInt(position.Number, 10) + "." +
		strconv.FormatInt(position.At.UnixMicro(), 10))
	return base64.RawURLEncoding.EncodeToString(append(c.tag(payload), payload...))
}

// Decode reads a position back; every failure is the same answer, as the other codecs give.
func (c PlatformEventCursorCodec) Decode(cursor string) (adminrepo.PlatformPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) < cursorTagLength+1 {
		return adminrepo.PlatformPosition{}, errPlatformCursorInvalid
	}
	tag, payload := raw[:cursorTagLength], raw[cursorTagLength:]
	if !hmac.Equal(tag, c.tag(payload)) {
		return adminrepo.PlatformPosition{}, errPlatformCursorInvalid
	}
	number, at, found := strings.Cut(string(payload), ".")
	if !found {
		return adminrepo.PlatformPosition{}, errPlatformCursorInvalid
	}
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil || n < 0 {
		return adminrepo.PlatformPosition{}, errPlatformCursorInvalid
	}
	micros, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return adminrepo.PlatformPosition{}, errPlatformCursorInvalid
	}
	return adminrepo.PlatformPosition{Number: n, At: time.UnixMicro(micros).UTC()}, nil
}

func (c PlatformEventCursorCodec) tag(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)[:cursorTagLength]
}
