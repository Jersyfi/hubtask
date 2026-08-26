// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package job

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Find(context.Context, shared.ID) (domain.Job, error) {
	return domain.Job{}, shared.ErrNotFound.WithDetail("jobs.not_found")
}

func (double) Cancel(context.Context, shared.ID, time.Time) (domain.Job, error) {
	return domain.Job{}, shared.ErrConflict.WithDetail("jobs.already_finished")
}

var _ Jobs = double{}

// Two refusals the use case tells apart, so both have to be expressible: a job that is not this
// tenant's - which is the same answer as one that never existed - and one that is over.
func TestTheTwoRefusalsAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown job is reported as %v", err)
	}

	_, err := (double{}).Cancel(t.Context(), "", time.Time{})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a job that is over is reported as %v", err)
	}
	if errors.Is(err, shared.ErrNotFound) {
		t.Error("a job that is over reads as one that never existed")
	}
}
