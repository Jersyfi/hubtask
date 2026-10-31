// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// refused is a delivery whose signature, token or shape does not hold.
func refused(why string) error {
	return shared.ErrNotFound.WithDetail("chatbot.delivery_refused").WithCause(errors.New("chat bot: " + why))
}

// revoked is a credential the platform no longer takes.
func revoked(why string) error {
	return shared.ErrForbidden.WithDetail("chatbot.token_revoked").WithCause(errors.New("chat bot: " + why))
}

// sameSecret compares in constant time; an empty secret proves nothing.
func sameSecret(given string, want secret.Secret) bool {
	return given != "" && !want.IsEmpty() && subtle.ConstantTimeCompare([]byte(given), []byte(want.Reveal())) == 1
}

func header(h map[string][]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// fields reads a body a platform sent as a form or as JSON into its top-level strings.
func fields(body []byte) (map[string]string, error) {
	out := map[string]string{}
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, err
		}
		for k, v := range doc {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
		return out, nil
	}
	form, err := url.ParseQuery(trimmed)
	if err != nil {
		return nil, err
	}
	for k := range form {
		out[k] = form.Get(k)
	}
	return out, nil
}

// markdown escapes what a title would otherwise format, link or mention: a title `@channel` must
// not call a whole channel.
func markdown(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '*', '_', '[', ']', '(', ')', '#', '`', '~', '<', '>', '|':
			b.WriteRune('\\')
			b.WriteRune(r)
		case '@':
			b.WriteString("@​")
		case '\n', '\r':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
