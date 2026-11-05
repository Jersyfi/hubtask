// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A malformed address is answered exactly as an unknown one, before anything is looked up or
// stored (UC-JUM-02 check 4, UC-JUM-04 check 4).
func TestAMalformedAddressIsTheSameNotFound(t *testing.T) {
	for _, presented := range []string{"", "not-a-token", "hti_", "hti_%%%"} {
		_, err := (WebhookIntake{}).Deliver(context.Background(), presented, "a@example.org", "s", "b")
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%q answered %v, want not found", presented, err)
			continue
		}
		if code := shared.AsError(err).DetailCode; code != "jumble.inbound_not_found" {
			t.Errorf("%q answered code %s", presented, code)
		}
	}
}
