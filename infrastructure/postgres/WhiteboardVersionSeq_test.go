// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A board version's place past what the column holds is no version: converted, it would wrap onto
// another one (2^32 + 1 is version 1), and naming or reading it would act on that one.
func TestABoardVersionPlacePastTheColumnIsNoVersion(t *testing.T) {
	board := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d101")
	for _, seq := range []int{0, -1, 1<<32 + 1} {
		err := WhiteboardRepository{}.NameVersion(context.Background(), board, seq, "Before the workshop")
		var e *shared.Error
		if !errors.As(err, &e) || e.DetailCode != "whiteboards.version_not_found" {
			t.Errorf("naming %d answered %v", seq, err)
		}
		if _, err := (WhiteboardRepository{}).Version(context.Background(), board, seq); !errors.As(err, &e) || e.DetailCode != "whiteboards.version_not_found" {
			t.Errorf("reading %d answered %v", seq, err)
		}
	}
}
