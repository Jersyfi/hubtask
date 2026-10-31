// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestOnlyAPublicReplySomebodyWroteIsMailedToTheRequesters(t *testing.T) {
	gone := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		comment     work.Comment
		byRequester bool
		want        bool
	}{
		{"a public reply", work.Comment{Body: "Fixed.", Kind: work.CommentByUser}, false, true},
		{"a reply of an older row without a kind", work.Comment{Body: "Fixed."}, false, true},
		{"an internal note", work.Comment{Body: "Ask Lena.", Internal: true}, false, false},
		{"the requester's own mail", work.Comment{Body: "Still broken."}, true, false},
		{"a system comment", work.Comment{Body: "x", Kind: work.CommentBySystem}, false, false},
		{"a deleted comment", work.Comment{DeletedAt: &gone}, false, false},
		{"an empty body", work.Comment{}, false, false},
	}
	for _, c := range cases {
		if got := MailsRequesters(c.comment, c.byRequester); got != c.want {
			t.Errorf("%s: mailed %v, want %v", c.name, got, c.want)
		}
	}
}
