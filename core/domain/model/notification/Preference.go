// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Preference is what one person has said about being told one kind of thing on one channel.
//
// A row is an exception rather than a setting. Its absence is the default and the default is on,
// which is what keeps a new category from needing a backfill across every account in every tenant -
// and what keeps "on" written down in one place rather than in a column default and a constant that
// will one day disagree.
type Preference struct {
	TenantID  shared.ID
	AccountID shared.ID
	Category  Category
	Channel   Channel
	Enabled   bool
	// IncludeTitle is whether the entry's title may travel in the message.
	//
	// True by default, which is what data-protection.md §9 means by "title and link only, no full
	// text; switchable": the minimum is the default and the switch takes even that away, leaving a
	// message that says something concerns you and where to look. There is no setting that adds the
	// note body - that is not a preference, it is a rule, and it is enforced in Content rather than
	// left to a column.
	IncludeTitle bool
	UpdatedAt    time.Time
}

// DefaultPreference is what somebody who has said nothing gets: told, with the title.
//
// A value rather than a zero struct, because the zero value of a bool is false and the default here
// is on - a caller that reached for `Preference{}` would silently mean the opposite of the default.
func DefaultPreference(tenantID, accountID shared.ID, category Category, channel Channel) Preference {
	return Preference{
		TenantID:     tenantID,
		AccountID:    accountID,
		Category:     category,
		Channel:      channel,
		Enabled:      true,
		IncludeTitle: true,
	}
}

// Recipient is what deciding needs to know about the person being told.
//
// Not the account aggregate: what a decision turns on is whether there is an address to send to and
// who they are, and taking the whole account would put a password hash in the argument list of a
// function about email preferences.
type Recipient struct {
	AccountID shared.ID
	// HasAddress is whether there is somewhere to send. An invited account that never arrived has
	// one; a service account does not.
	HasAddress bool
}

// Decision is whether to send, and what may travel if so.
type Decision struct {
	// Send is whether a message goes out. False leaves a record in SUPPRESSED with Reason set.
	Send bool
	// Reason is the detail code for a decision not to send. Empty when Send is true.
	Reason string
	// IncludeTitle is whether the renderer may put the entry's title in the message. Meaningless
	// when Send is false, and false then, so a caller that ignores Send cannot leak a title.
	IncludeTitle bool
}

// Decide judges one notification against the recipient and their preference.
//
// The order is not arbitrary. Self-caused first, because being told about your own action is noise
// whatever anybody's preferences say; then the missing address, because a preference about a
// channel that cannot reach you is moot; then the preference itself. Each is a different answer to
// "why did I hear nothing", and the record keeps whichever applied.
//
// Authorisation is not decided here and must not be: whether this recipient may see this entry at
// all is the application layer's (ADR-0005, rule 2). What this decides is whether somebody who may
// see it wants to hear about it.
func Decide(n Notification, recipient Recipient, preference Preference) Decision {
	if !n.ActorID.IsZero() && n.ActorID == recipient.AccountID {
		return Decision{Reason: ReasonSelfCaused}
	}
	if !recipient.HasAddress {
		return Decision{Reason: ReasonNoAddress}
	}
	if n.Category.Suppressible() && !preference.Enabled {
		return Decision{Reason: ReasonCategoryOff}
	}
	return Decision{Send: true, IncludeTitle: preference.IncludeTitle}
}
