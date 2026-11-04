// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A version's place past what the column holds is no version: converted, it would wrap onto
// another one (2^32 + 1 is version 1), and naming or reading it would act on that one.
func TestAVersionPlacePastTheColumnIsNoVersion(t *testing.T) {
	page := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d001")
	for _, seq := range []int{0, -1, 1<<32 + 1} {
		err := PageRepository{}.NameVersion(context.Background(), page, seq, "Sent to legal")
		var e *shared.Error
		if !errors.As(err, &e) || e.DetailCode != "pages.version_not_found" {
			t.Errorf("naming %d answered %v", seq, err)
		}
		if _, err := (PageRepository{}).Version(context.Background(), page, seq); !errors.As(err, &e) || e.DetailCode != "pages.version_not_found" {
			t.Errorf("reading %d answered %v", seq, err)
		}
	}
}
