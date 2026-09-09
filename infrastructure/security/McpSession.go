// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// mcpSessionInfo is the domain-separation label (see TokenHasher): a session identifier can never
// be replayed as a media token, a page cursor, or anything else derived from the same installation
// secret.
const mcpSessionInfo = "hubtask/mcp-session/v1"

// mcpSessionTagLength truncates the tag to 128 bits, with the page cursor's justification.
const mcpSessionTagLength = 16

// McpSessionLifetime is how long a session identifier stays usable.
//
// Long enough that an agent working through a task does not re-handshake in the middle of it, short
// enough that an identifier copied out of a log is useless by the time somebody reads the log. It
// is not a credential - a request carrying one is authenticated by its bearer token like every
// other request, and the session says only *which conversation* the request belongs to - so the
// lifetime is about tidiness rather than about containment.
const McpSessionLifetime = 12 * time.Hour

// ErrMcpSessionUnknown is every way a presented session identifier is not one this server minted
// for this actor: a forgery, a truncation, one that has expired, and one belonging to somebody
// else. **One answer for all four**, deliberately - saying which check failed tells whoever is
// probing how far their forgery got, which is the page cursor's reasoning applied to a session.
var ErrMcpSessionUnknown = errors.New("unknown MCP session")

// McpSessionIssuer mints and checks the `Mcp-Session-Id` of the MCP transport (J-13).
//
// **A signed statement rather than a row**, and that is the decision worth reading. MCP lets a
// server hand a client a session identifier at `initialize` and expects it back on every later
// request; the obvious implementation is a map in the process. This installation runs the `api`
// role stateless and horizontally scaled (ADR-0046), so a map would bind a client to whichever pod
// answered its handshake - and the next request, load-balanced elsewhere, would be told its session
// does not exist. A client cannot recover from that: it re-handshakes, lands on a third pod, and
// the failure looks like a flapping server.
//
// So the identifier carries what it asserts - the workspace, the account, and when it was minted -
// and an HMAC over the three. Any pod can then check it with no shared state, which is the same
// property the media token and the page cursor already have and for the same reason.
//
// It is not a credential and does not authenticate anything. Every request that carries one is
// authenticated by its bearer token first; the session is checked *against that actor*, so a
// session belonging to somebody else is refused even when presented with a perfectly good token.
type McpSessionIssuer struct {
	key []byte
}

// NewMcpSessionIssuer derives the signing key from the installation secret, under this token's own
// label.
func NewMcpSessionIssuer(installationSecret secret.Secret) McpSessionIssuer {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(mcpSessionInfo))
	return McpSessionIssuer{key: mac.Sum(nil)}
}

// Issue mints the identifier a handshake answers with.
func (i McpSessionIssuer) Issue(tenantID, accountID shared.ID, now time.Time) string {
	issued := now.Unix()

	session := make([]byte, 0, mcpSessionTagLength+8+len(tenantID)+1+len(accountID))
	session = append(session, i.tag(tenantID, accountID, issued)...)
	session = binary.BigEndian.AppendUint64(session, uint64(issued)) //nolint:gosec // G115: a unix timestamp fits until the year 292277026596
	session = append(session, (tenantID.String() + "\x00" + accountID.String())...)
	return base64.RawURLEncoding.EncodeToString(session)
}

// Validate judges a presented identifier against the actor presenting it.
//
// The identity in the token is compared with the identity the request authenticated as, rather than
// trusted from the token: a session says which conversation a request belongs to, and a request
// that authenticated as one person may not continue another's.
func (i McpSessionIssuer) Validate(
	session string, tenantID, accountID shared.ID, now time.Time,
) error {
	raw, err := base64.RawURLEncoding.DecodeString(session)
	if err != nil || len(raw) <= mcpSessionTagLength+8 {
		return ErrMcpSessionUnknown
	}

	issued := int64(binary.BigEndian.Uint64(raw[mcpSessionTagLength : mcpSessionTagLength+8])) //nolint:gosec // G115: the value was written by Issue
	claimed := string(raw[mcpSessionTagLength+8:])

	// The tag is verified before anything the token says is used, so a caller can never act on an
	// identity nobody signed for.
	if !hmac.Equal(raw[:mcpSessionTagLength], i.tagOf(claimed, issued)) {
		return ErrMcpSessionUnknown
	}
	if now.Unix() > issued+int64(McpSessionLifetime.Seconds()) {
		return ErrMcpSessionUnknown
	}
	if claimed != tenantID.String()+"\x00"+accountID.String() {
		return ErrMcpSessionUnknown
	}
	return nil
}

func (i McpSessionIssuer) tag(tenantID, accountID shared.ID, issued int64) []byte {
	return i.tagOf(tenantID.String()+"\x00"+accountID.String(), issued)
}

func (i McpSessionIssuer) tagOf(identity string, issued int64) []byte {
	mac := hmac.New(sha256.New, i.key)
	mac.Write([]byte(identity + "\x00" + strconv.FormatInt(issued, 10)))
	return mac.Sum(nil)[:mcpSessionTagLength]
}
