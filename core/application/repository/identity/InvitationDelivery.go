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
}

// InvitationJobs reads the queue's last invitation job per account, of the transaction's
// workspace.
type InvitationJobs interface {
	Latest(ctx context.Context, accountIDs []shared.ID) ([]InvitationJob, error)
}
