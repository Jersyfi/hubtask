// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import "github.com/Jersyfi/hubtask/core/domain/model/work"

// RequesterAddress is a requester without an account, by the address their mail came from and the
// name it gave (accepted.md B).
type RequesterAddress struct {
	Address string
	Name    string
}

// MaxRequesterAddresses bounds who one reply is mailed to: a request is filed for a person, and
// a long list is a mail thread with many copies, not a requester.
const MaxRequesterAddresses = 20

// MailsRequesters says whether a comment goes by mail to the requesters an entry was filed for
// (UC-SVC-12): a public reply somebody wrote. Never an internal note, a reply under one, a system
// comment or a deleted one - and never the requester's own words the mail intake filed, which
// would answer them with themselves.
func MailsRequesters(comment work.Comment, byRequester bool) bool {
	if byRequester || comment.Internal || comment.DeletedAt != nil {
		return false
	}
	if comment.Kind != "" && comment.Kind != work.CommentByUser {
		return false
	}
	return comment.Body != ""
}
