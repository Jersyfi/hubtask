// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/port/wopi"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// wopiTokenInfo is the domain-separation label: a WOPI access token is never a media token, a
// cursor or anything else derived from the installation secret (security.md §8).
//
//nolint:gosec // G101: a public derivation label, not a credential
const wopiTokenInfo = "hubtask/wopi/v1"

// wopiIDLength is a UUID's text form; the token carries two.
const wopiIDLength = 36

// WopiTokenIssuer signs the access tokens a document server presents to the WOPI host: one
// person, one file, read or write, until an expiry (security.md §8). Signed, never stored, never
// logged (rule 10) - the tenant and the person travel in it in clear and under the tag, as the
// media token's tenant does, because the WOPI routes carry no other credential.
type WopiTokenIssuer struct {
	key []byte
}

var _ wopi.Tokens = WopiTokenIssuer{}

// NewWopiTokenIssuer derives the signing key from the installation secret under its own label.
func NewWopiTokenIssuer(installationSecret secret.Secret) WopiTokenIssuer {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(wopiTokenInfo))
	return WopiTokenIssuer{key: mac.Sum(nil)}
}

// Issue mints the token for a grant.
func (i WopiTokenIssuer) Issue(grant wopi.Grant) string {
	expiry := grant.ExpiresAt.Unix()
	tag := i.tag(grant.TenantID, grant.AccountID, grant.MediaID, grant.Write, expiry)
	token := make([]byte, 0, len(tag)+9+2*wopiIDLength)
	token = append(token, tag...)
	token = binary.BigEndian.AppendUint64(token, uint64(expiry)) //nolint:gosec // G115: a unix timestamp
	token = append(token, writeByte(grant.Write))
	token = append(token, grant.TenantID.String()...)
	token = append(token, grant.AccountID.String()...)
	return base64.RawURLEncoding.EncodeToString(token)
}

// Read verifies a token presented for a file and answers its grant. Every forgery is one
// answer; an expired token is refused alike - the document server asks the person to open the
// file again.
func (i WopiTokenIssuer) Read(token string, mediaID shared.ID, now time.Time) (wopi.Grant, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	head := mediaTokenTagLength + 9
	if err != nil || len(raw) != head+2*wopiIDLength {
		return wopi.Grant{}, wopi.ErrInvalidToken()
	}
	expiry := int64(binary.BigEndian.Uint64(raw[mediaTokenTagLength : mediaTokenTagLength+8])) //nolint:gosec // G115: written by Issue
	write := raw[mediaTokenTagLength+8] == 1
	tenantID := shared.ID(raw[head : head+wopiIDLength])
	accountID := shared.ID(raw[head+wopiIDLength:])
	if !hmac.Equal(raw[:mediaTokenTagLength], i.tag(tenantID, accountID, mediaID, write, expiry)) || now.Unix() > expiry {
		return wopi.Grant{}, wopi.ErrInvalidToken()
	}
	return wopi.Grant{
		TenantID: tenantID, AccountID: accountID, MediaID: mediaID, Write: write,
		ExpiresAt: time.Unix(expiry, 0).UTC(),
	}, nil
}

func (i WopiTokenIssuer) tag(tenantID, accountID, mediaID shared.ID, write bool, expiry int64) []byte {
	mac := hmac.New(sha256.New, i.key)
	mac.Write([]byte(tenantID.String() + "\x00" + accountID.String() + "\x00" + mediaID.String() +
		"\x00" + strconv.FormatBool(write) + "\x00" + strconv.FormatInt(expiry, 10)))
	return mac.Sum(nil)[:mediaTokenTagLength]
}

func writeByte(write bool) byte {
	if write {
		return 1
	}
	return 0
}
