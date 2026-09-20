// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// A check job without a tenant is a programming error, not a job that quietly does nothing: every
// read the check makes is made for one workspace.
func TestACheckJobNeedsItsTenant(t *testing.T) {
	_, err := AutomationCheck{}.Run(context.Background(), queue.Job{Kind: queue.KindAutomationCheck})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error %v, want ErrInternal", err)
	}
}
