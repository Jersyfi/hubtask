// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import "encoding/base64"

// SignStreamPayload is a cursor with this codec's tag over a payload the test spells itself - the
// shape a cursor minted by an earlier build has, which Encode can no longer produce.
func SignStreamPayload(c StreamCursorCodec, payload string) string {
	raw := []byte(payload)
	return base64.RawURLEncoding.EncodeToString(append(c.tag(raw), raw...))
}
