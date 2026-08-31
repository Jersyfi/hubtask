// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordRetentionWarning tells the people who can stop it that a rule is about to act
// (data-retention.md §6, R-1 answered in G-12).
//
// The notification path C-09 built rather than a channel of its own: the preference is honoured,
// the record is deduplicated on the entry, and the send is a job like every other. What is
// different is only what caused it - not somebody's action and no domain event, but a retention
// pass concluding that an object's period has run out.
//
// # Why it carries no event
//
// `event_id` is null, which the column has allowed since C-09 "for the invitation, which is not an
// event". A marking is the third of those: it is a decision the engine made about a row, and there
// is no envelope that says so. What makes it happen once is the marking itself - an entry is
// marked once and the phase that marks it does not see it again.
//
// # Who is told
//
// The rule says, in the vocabulary §2 gives it: the entry's own members, the administrators of the
// container it sits in, the administrators of the workspace. The first is a list the entry carries;
// the other two are the role matrix's answer, read along the entry's path, and neither of them is a
// list a rule's author can widen - a recipient outside the closed set is refused where the rule is
// written.
type RecordRetentionWarning struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Memberships   identityrepo.Memberships
	Members       Members
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// RetentionWarning is one marked entry and what the rule said about warning people.
type RetentionWarning struct {
	TenantID shared.ID
	// ItemID is the entry the warning is about, and what the message links to.
	ItemID shared.ID
	// Path is the entry's chain - tenant, hub, collection - which is what the administrators are
	// resolved along.
	Path []identity.Scope
	// Recipients is the rule's audience, in the closed set of data-retention.md §2.
	Recipients []lifecycle.Recipient
}

// Warn writes one record per person and queues the deliveries.
//
// It runs inside the caller's transaction - the sweep's marking write - so the warnings and the
// marking they are about commit together. A warning that committed without the marking would tell
// somebody their work is going when nothing had decided that; a marking that committed without its
// warnings would take the entry through its grace period in silence, which is the failure §6 is
// about.
//
// One person who is both an entry member and an administrator is told once. The record is per
// person and per entry, not per reason they qualify.
func (r RecordRetentionWarning) Warn(ctx context.Context, warning RetentionWarning) error {
	if warning.ItemID.IsZero() || len(warning.Recipients) == 0 {
		return nil
	}

	recipients, err := r.resolve(ctx, warning)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := r.one(ctx, warning, recipient); err != nil {
			return err
		}
	}
	return nil
}

// resolve turns the rule's vocabulary into accounts, without duplicates.
func (r RecordRetentionWarning) resolve(
	ctx context.Context, warning RetentionWarning,
) ([]shared.ID, error) {
	seen := map[shared.ID]bool{}
	var recipients []shared.ID
	add := func(ids []shared.ID) {
		for _, id := range ids {
			if id.IsZero() || seen[id] {
				continue
			}
			seen[id] = true
			recipients = append(recipients, id)
		}
	}

	for _, recipient := range warning.Recipients {
		switch recipient {
		case lifecycle.RecipientItemMembers:
			if r.Members == nil {
				continue
			}
			members, err := r.Members.List(ctx, warning.ItemID)
			if err != nil {
				return nil, err
			}
			add(members)
		case lifecycle.RecipientCollectionAdmins:
			// Along the entry's whole path, because an administrator of the hub administers the
			// collections under it: the role applies downwards (domain-model.md §3.2), and a
			// resolution that looked only at the collection would leave out exactly the people a
			// workspace with one administrator has.
			administrators, err := r.Memberships.Administrators(ctx, warning.Path)
			if err != nil {
				return nil, err
			}
			add(administrators)
		case lifecycle.RecipientTenantAdmins:
			administrators, err := r.Memberships.Administrators(
				ctx, []identity.Scope{identity.TenantScope()})
			if err != nil {
				return nil, err
			}
			add(administrators)
		}
	}
	return recipients, nil
}

// one writes the record for a single person and queues its delivery.
func (r RecordRetentionWarning) one(
	ctx context.Context, warning RetentionWarning, recipientID shared.ID,
) error {
	written, err := domain.New(domain.NewInput{
		ID:          r.IDs.NewID(),
		TenantID:    warning.TenantID,
		RecipientID: recipientID,
		Category:    domain.CategoryRetention,
		Channel:     domain.ChannelEmail,
		ItemID:      warning.ItemID,
		At:          r.Clock.Now(),
	})
	if err != nil {
		return err
	}

	decision, err := decideFor(
		ctx, r.Accounts, r.Preferences, written, recipientID, domain.CategoryRetention)
	if err != nil {
		return err
	}
	if !decision.Send {
		written = written.Suppress(decision.Reason)
	}

	first, err := r.Notifications.Insert(ctx, written)
	if err != nil {
		return err
	}
	if !first || !decision.Send {
		return nil
	}

	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindNotificationDeliver,
		TenantID: written.TenantID,
		// The record's own identifier, so a retried write cannot queue a second send of one
		// message - the same key every other recorder uses.
		DedupeKey: written.ID.String(),
		RunAt:     r.Clock.Now(),
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return err
}
