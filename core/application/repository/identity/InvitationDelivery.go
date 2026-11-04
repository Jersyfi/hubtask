// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// InvitationJob is the last invitation mail queued for one account, as the queue holds it: the
// job's state in the queue's words, its last failure's code and parameters, and its times.
type InvitationJob struct {
	AccountID  shared.ID
	State      string
	Code       string
	Params     map[string]string
	QueuedAt   time.Time
	FinishedAt time.Time
	// MailState, MailReason and MailSentAt are the newest invitation notification written since
	// the job: what the mail itself did once the job recorded it - PENDING, SENT, FAILED or
	// SUPPRESSED, with the record's reason code. Empty where none was written.
	MailState  string
	MailReason string
	MailSentAt time.Time
}

// InvitationJobs reads the queue's last invitation job per account, of the transaction's
// workspace.
type InvitationJobs interface {
	Latest(ctx context.Context, accountIDs []shared.ID) ([]InvitationJob, error)
	// CancelOpen cancels the invitation mail still waiting for one account and sets its unsent
	// record aside, and answers how many jobs it cancelled: a handed-over link must not be
	// replaced by the token a later delivery would mint (identity.md §12, "Never both"). A mail
	// being sent right now cannot be stopped and is `accounts.invitation_mail_sending`.
	CancelOpen(ctx context.Context, accountID shared.ID, now time.Time) (int, error)
}
