// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// HistoryPage is one page of an object's history, newest first.
type HistoryPage struct {
	Steps []cmdb.Step
	Info  workrepo.PageInfo
}

// History keeps an object's own history (migration 0435): appended in the write's transaction,
// read newest first.
type History interface {
	AppendCIHistory(ctx context.Context, step cmdb.Step) error
	CIHistory(ctx context.Context, ciID shared.ID, page workrepo.Page) (HistoryPage, error)
}
