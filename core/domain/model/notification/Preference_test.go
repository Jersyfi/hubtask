// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The default is on, and it is a value rather than a zero struct - the zero value of a bool is
// false, which is the opposite of what somebody who has said nothing gets.
func TestSayingNothingMeansBeingTold(t *testing.T) {
	preference := notification.DefaultPreference(
		tenant, recipient, notification.CategoryComment, notification.ChannelEmail)

	if !preference.Enabled {
		t.Error("the default is off - somebody who has said nothing would hear nothing")
	}
	if !preference.IncludeTitle {
		t.Error("the default withholds the title, and data-protection.md §9 makes it the minimum")
	}
	if preference.TenantID != tenant || preference.AccountID != recipient {
		t.Errorf("the default belongs to nobody: %+v", preference)
	}

	var zero notification.Preference
	if zero.Enabled {
		t.Error("the zero value is on - then nothing forces a caller through DefaultPreference")
	}
}

func record(t *testing.T, change func(*notification.NewInput)) notification.Notification {
	t.Helper()
	in := input()
	change(&in)
	written, err := notification.New(in)
	if err != nil {
		t.Fatalf("writing the record: %v", err)
	}
	return written
}

func TestDecidingWhetherSomebodyIsTold(t *testing.T) {
	on := notification.DefaultPreference(
		tenant, recipient, notification.CategoryComment, notification.ChannelEmail)
	off := on
	off.Enabled = false
	quiet := on
	quiet.IncludeTitle = false

	reachable := notification.Recipient{AccountID: recipient, HasAddress: true}

	for _, tc := range []struct {
		name         string
		change       func(*notification.NewInput)
		recipient    notification.Recipient
		preference   notification.Preference
		send         bool
		reason       string
		includeTitle bool
	}{
		{
			name: "somebody else wrote it", change: func(*notification.NewInput) {},
			recipient: reachable, preference: on, send: true, includeTitle: true,
		},
		{
			name:      "the recipient wrote it themselves",
			change:    func(in *notification.NewInput) { in.ActorID = recipient },
			recipient: reachable, preference: on,
			reason: notification.ReasonSelfCaused,
		},
		{
			name: "there is nowhere to send", change: func(*notification.NewInput) {},
			recipient: notification.Recipient{AccountID: recipient}, preference: on,
			reason: notification.ReasonNoAddress,
		},
		{
			name: "the category is switched off", change: func(*notification.NewInput) {},
			recipient: reachable, preference: off,
			reason: notification.ReasonCategoryOff,
		},
		{
			name: "the title is withheld", change: func(*notification.NewInput) {},
			recipient: reachable, preference: quiet, send: true, includeTitle: false,
		},
		{
			// The invitation ignores the switch, because the switch is behind the door it locks.
			name: "an invitation with everything switched off",
			change: func(in *notification.NewInput) {
				in.Category = notification.CategoryInvitation
				in.ActorID = actor
			},
			// The title flag still comes from the row; an invitation has no entry to name
			// anyway, so what it governs here is nothing.
			recipient: reachable, preference: off, send: true, includeTitle: true,
		},
		{
			// And still not to somebody who invited themselves - the order matters.
			name: "an invitation the recipient caused",
			change: func(in *notification.NewInput) {
				in.Category = notification.CategoryInvitation
				in.ActorID = recipient
			},
			recipient: reachable, preference: on,
			reason: notification.ReasonSelfCaused,
		},
		{
			// Nobody caused it: the automatic assignment acts for the system (C-02), and a zero
			// actor must not match a zero recipient into a self-caused suppression.
			name:       "nobody caused it",
			change:     func(in *notification.NewInput) { in.ActorID = "" },
			recipient:  notification.Recipient{AccountID: shared.ID(""), HasAddress: true},
			preference: on, send: true, includeTitle: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := notification.Decide(record(t, tc.change), tc.recipient, tc.preference)

			if decision.Send != tc.send {
				t.Errorf("send %v, want %v (reason %q)", decision.Send, tc.send, decision.Reason)
			}
			if decision.Reason != tc.reason {
				t.Errorf("reason %q, want %q", decision.Reason, tc.reason)
			}
			if decision.IncludeTitle != tc.includeTitle {
				t.Errorf("includeTitle %v, want %v", decision.IncludeTitle, tc.includeTitle)
			}
		})
	}
}

// A caller that ignores Send and reads IncludeTitle must not be handed a licence to print a title.
func TestASuppressedDecisionNeverPermitsATitle(t *testing.T) {
	off := notification.DefaultPreference(
		tenant, recipient, notification.CategoryComment, notification.ChannelEmail)
	off.Enabled = false

	decision := notification.Decide(
		record(t, func(*notification.NewInput) {}),
		notification.Recipient{AccountID: recipient, HasAddress: true},
		off)

	if decision.IncludeTitle {
		t.Error("a suppressed decision permits the title - a careless caller would leak one")
	}
}
