// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type double struct {
	appended []event.Envelope
	err      error
}

func (d *double) Append(_ context.Context, envelope event.Envelope) error {
	if d.err != nil {
		return d.err
	}
	d.appended = append(d.appended, envelope)
	return nil
}

var _ Events = (*double)(nil)

// The one behaviour the port promises: a failure is reported rather than swallowed. An event
// dropped quietly is a change that automation and webhooks never hear about, and nothing later
// can tell that it is missing.
func TestAFailureIsReportedRatherThanSwallowed(t *testing.T) {
	failing := &double{err: shared.ErrUnavailable.WithDetail("postgres.query_failed")}

	err := failing.Append(t.Context(), event.Envelope{Type: event.ContainerCreated})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("error %v, want the failure to reach the caller", err)
	}
	if len(failing.appended) != 0 {
		t.Error("a failed append recorded something")
	}
}
