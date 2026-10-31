// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/media"
	mediadomain "github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// While files stay held - here, with no scanner wired, they always do - the job comes back; with
// nothing held it ends, and the next held confirmation seeds it again.
func TestTheScanJobRepeatsWhileFilesAreHeld(t *testing.T) {
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	records := &reclaimable{held: []mediadomain.Object{{ID: "0192f000-0000-7000-8000-0000000000b1", Status: mediadomain.StatusPendingScan}}}
	job := MediaScan{Scan: media.ScanHeldMedia{Objects: records, UnitOfWork: &directWork{}}}
	var _ queue.Detached = job

	result, err := job.Run(t.Context(), queue.Job{TenantID: tenant, Kind: queue.KindMediaScan})
	if err != nil || !result.Repeat || result.RepeatAfter <= 0 {
		t.Fatalf("with a file held: %+v, %v", result, err)
	}
	records.held = nil
	result, err = job.Run(t.Context(), queue.Job{TenantID: tenant, Kind: queue.KindMediaScan})
	if err != nil || result.Repeat {
		t.Fatalf("with nothing held: %+v, %v", result, err)
	}
	if _, err := job.Run(t.Context(), queue.Job{Kind: queue.KindMediaScan}); err == nil {
		t.Error("a scan without a tenant ran")
	}
}
