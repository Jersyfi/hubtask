// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// mediaTokenInfo is the domain-separation label (see TokenHasher): a media token can never be
// replayed as a page cursor or anything else derived from the same installation secret.
//
//nolint:gosec // G101: a public derivation label, not a credential - the secret is the installation's
const mediaTokenInfo = "hubtask/media-content/v1"

// mediaTokenTagLength truncates the tag to 128 bits, with cursorTagLength's justification.
const mediaTokenTagLength = 16

// MediaTokenPurpose separates the two directions: a token minted for an upload must not open a
// download, and the purpose is signed so it cannot be flipped.
type MediaTokenPurpose string

const (
	MediaTokenUpload   MediaTokenPurpose = "upload"
	MediaTokenDownload MediaTokenPurpose = "download"
)

// MediaTokenIssuer signs the content routes' capability tokens (C-06): on a local-storage
// installation, /media/{id}:content plays the part a presigned URL plays on an object-storage
// one, and the token is what makes the URL the credential - bound to one object, one direction,
// and an expiry.
type MediaTokenIssuer struct {
	key []byte
}

// NewMediaTokenIssuer derives the signing key from the installation secret, under this token's
// own label.
func NewMediaTokenIssuer(installationSecret secret.Secret) MediaTokenIssuer {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(mediaTokenInfo))
	return MediaTokenIssuer{key: mac.Sum(nil)}
}

// Issue mints the token for one object, one direction, until expiresAt.
//
// The tenant travels in the token, in clear and signed. The content routes carry no bearer
// credential - the URL is the capability, exactly as a presigned bucket URL is - so there is
// nothing else on the request that could say which tenant to open the transaction as, and a
// tenant read from the path or a header would be a way around row level security
// (multi-tenancy.md §2.2). Signed rather than merely carried: swapping it invalidates the tag.
func (i MediaTokenIssuer) Issue(
	purpose MediaTokenPurpose, tenantID, mediaID shared.ID, expiresAt time.Time,
) string {
	payload := i.payload(purpose, tenantID, mediaID, expiresAt.Unix())

	mac := hmac.New(sha256.New, i.key)
	mac.Write(payload)
	tag := mac.Sum(nil)[:mediaTokenTagLength]

	token := make([]byte, 0, len(tag)+8+len(tenantID))
	token = append(token, tag...)
	token = binary.BigEndian.AppendUint64(token, uint64(expiresAt.Unix())) //nolint:gosec // G115: a unix timestamp fits until the year 292277026596
	token = append(token, tenantID.String()...)
	return base64.RawURLEncoding.EncodeToString(token)
}

// Validate judges a presented token. A forgery of any kind is one indistinguishable answer, for
// the reason a page cursor's is: saying which check failed tells whoever is probing how far
// their forgery got. Expiry is the one distinguished refusal - an expired token is a token this
// server really minted, and "stage the upload again" is actionable where "invalid" is not.
// It returns the tenant the token was minted in, which is what the caller opens its transaction
// as. The value is only ever returned after the tag has been verified, so a caller cannot be
// handed a tenant nobody signed for.
func (i MediaTokenIssuer) Validate(
	token string, purpose MediaTokenPurpose, mediaID shared.ID, now time.Time,
) (shared.ID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= mediaTokenTagLength+8 {
		return "", errMediaTokenInvalid()
	}

	expiry := int64(binary.BigEndian.Uint64(raw[mediaTokenTagLength : mediaTokenTagLength+8])) //nolint:gosec // G115: the value was written by Issue
	tenantID := shared.ID(raw[mediaTokenTagLength+8:])
	payload := i.payload(purpose, tenantID, mediaID, expiry)

	mac := hmac.New(sha256.New, i.key)
	mac.Write(payload)
	if !hmac.Equal(raw[:mediaTokenTagLength], mac.Sum(nil)[:mediaTokenTagLength]) {
		return "", errMediaTokenInvalid()
	}
	if now.Unix() > expiry {
		return "", shared.ErrValidation.
			WithDetail("media.upload_expired").
			WithFields(shared.FieldError{Path: "/token", Code: "media.upload_expired"})
	}
	return tenantID, nil
}

func (i MediaTokenIssuer) payload(
	purpose MediaTokenPurpose, tenantID, mediaID shared.ID, expiry int64,
) []byte {
	return []byte(string(purpose) + "\x00" + tenantID.String() + "\x00" + mediaID.String() +
		"\x00" + strconv.FormatInt(expiry, 10))
}

func errMediaTokenInvalid() error {
	return shared.ErrValidation.
		WithDetail("media.token_invalid").
		WithFields(shared.FieldError{Path: "/token", Code: "media.token_invalid"})
}

// ValidateUpload and ValidateDownload are Validate with the purpose named rather than passed.
//
// They exist so that the REST layer can hold this as an interface of its own without knowing the
// purpose type - presentation may not import infrastructure - and they make the one mistake that
// matters impossible to write: a handler cannot pass the wrong direction if it cannot pass one.
func (i MediaTokenIssuer) ValidateUpload(
	token string, mediaID shared.ID, now time.Time,
) (shared.ID, error) {
	return i.Validate(token, MediaTokenUpload, mediaID, now)
}

func (i MediaTokenIssuer) ValidateDownload(
	token string, mediaID shared.ID, now time.Time,
) (shared.ID, error) {
	return i.Validate(token, MediaTokenDownload, mediaID, now)
}
