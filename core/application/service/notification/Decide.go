// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// decideFor reads what the person said and asks the domain whether they are told.
//
// One function for every path that writes a record - the outbox consumer and the reminder that
// fires (D-03) - because a preference honoured on one path and forgotten on another is a setting
// that works until it matters. The invitation is the deliberate exception and does not come
// through here: it is the one category no preference may switch off, and the setting that would
// switch it off sits behind the door that message unlocks.
func decideFor(
	ctx context.Context, accounts identityrepo.Accounts, preferences repository.Preferences,
	written domain.Notification, recipientID shared.ID, category domain.Category,
) (domain.Decision, error) {
	account, err := accounts.Find(ctx, recipientID)
	if errors.Is(err, shared.ErrNotFound) {
		// The account went between the cause and the record. There is nowhere to send, which is
		// what the domain calls it - and no record either, because the foreign key would refuse
		// one. Reported as a decision so the caller's insert fails cleanly rather than here.
		return domain.Decision{Reason: domain.ReasonNoAddress}, nil
	}
	if err != nil {
		return domain.Decision{}, err
	}

	preference, err := preferences.Find(ctx, recipientID, category, domain.ChannelEmail)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// Saying nothing is the default, and the default is on. Written down once, in the domain.
		preference = domain.DefaultPreference(
			written.TenantID, recipientID, category, domain.ChannelEmail)
	case err != nil:
		return domain.Decision{}, err
	}

	return domain.Decide(written, domain.Recipient{
		AccountID:  recipientID,
		HasAddress: account.Email != "",
	}, preference), nil
}
