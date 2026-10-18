// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A job that runs again after it wrote the archive tells the platform once, in the workspace's own
// scope, with the export's id.
func TestAnExportFinishedIsToldOnce(t *testing.T) {
	tenants := &tenantsStore{record: newShiftFixture(domain.TenantActive).tenants.record}
	recorder, log := recorderOn(tenants)
	work := &unitOfWork{}
	told := TellExportFinished{Platform: recorder, UnitOfWork: work, Clock: clock.Fixed(now)}
	export := shared.ID("0192f000-0000-7000-8000-0000000000f1")

	for range 2 {
		if err := told.Finished(t.Context(), lifecycleTenant, export); err != nil {
			t.Fatal(err)
		}
	}
	finished := onlyEvent(t, log, event.WorkspaceExportFinished)
	if finished.Payload["export_id"] != export.String() {
		t.Errorf("told %v", finished.Payload)
	}
	if work.scopes[0] != (persistence.Scope{TenantID: lifecycleTenant}) {
		t.Errorf("told under %+v", work.scopes[0])
	}

	if err := (TellExportFinished{}).Finished(t.Context(), lifecycleTenant, export); err != nil {
		t.Errorf("a build without the feed answered %v", err)
	}
}
