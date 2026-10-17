// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/platform"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// TellExportFinished tells a purchase platform that a workspace's export has been written
// (UC-INS-14). The export's job runs outside a transaction and may run again after it wrote the
// archive, so the event is written in a transaction of its own and keyed by the export: once,
// however often the job runs.
type TellExportFinished struct {
	Platform   platform.Recorder
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Finished writes `export_finished` for the export, in the workspace's own scope.
func (t TellExportFinished) Finished(ctx context.Context, workspace, exportID shared.ID) error {
	if !t.Platform.On() {
		return nil
	}
	return t.UnitOfWork.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
		_, err := t.Platform.RecordOnce(ctx, event.WorkspaceExportFinished, event.Actor{Kind: shared.ActorSystem},
			t.Clock.Now(), map[string]any{"export_id": exportID.String()}, "export:"+exportID.String())
		return err
	})
}
