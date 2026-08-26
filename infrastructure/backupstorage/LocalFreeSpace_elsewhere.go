// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build !linux && !darwin

package backupstorage

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
)

// FreeBytes has no answer on a platform this build does not know the statfs struct of. The port
// makes the report optional precisely so that this becomes a null in the connection probe rather
// than a number nobody measured.
func (s LocalStore) FreeBytes(context.Context) (int64, error) {
	return 0, shared.ErrUnavailable.WithDetail(port.CodeTargetFailed)
}
