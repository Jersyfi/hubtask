// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A workspace's week start is its own and may be cleared again, so that the installation's default
// is one a workspace can differ from and come back to (UC-INS-09 checks 4 and 5); a day none of the
// three is refused at its field, before anything is written.
func TestAWorkspaceSetsAndClearsItsOwnWeekStart(t *testing.T) {
	f := newWorkspaceFixture(at())
	update := func(day string) (domain.Workspace, error) {
		return (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
			UpdateWorkspaceCommand{Change: domain.WorkspaceChange{DefaultWeekStart: &day}})
	}

	changed, err := update("SUNDAY")
	if err != nil {
		t.Fatalf("setting the week start: %v", err)
	}
	if changed.Settings.WeekStart != domain.WeekStartSunday {
		t.Errorf("the week start reads %q", changed.Settings.WeekStart)
	}
	if got := workspaceOutput(changed, nil, nil)["default_week_start"]; got != "SUNDAY" {
		t.Errorf("the answer names %v", got)
	}
	if len(f.audit.entries) != 1 {
		t.Errorf("%d audit entries for one change", len(f.audit.entries))
	}

	cleared, err := update("")
	if err != nil {
		t.Fatalf("clearing the week start: %v", err)
	}
	if cleared.Settings.WeekStart != "" {
		t.Errorf("a cleared week start reads %q", cleared.Settings.WeekStart)
	}
	if _, held := workspaceOutput(cleared, nil, nil)["default_week_start"]; held {
		t.Error("a cleared week start is still answered")
	}

	writes := len(f.store.updates)
	_, err = update("WEDNESDAY")
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "workspace.week_start_invalid" ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/default_week_start" {
		t.Fatalf("a Wednesday answered %v", err)
	}
	if len(f.store.updates) != writes {
		t.Error("a refused day was written")
	}
}

// The workspace's answer carries what the installation set as the default language, zone and week
// start, only what it decided, so the screen can show it beside the workspace's own fields.
func TestTheWorkspaceAnswerNamesTheInstallationsDefaults(t *testing.T) {
	f := newWorkspaceFixture(at())
	read, err := (ReadWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	resolved := &ResolvedPolicy{InstallationLocalisation: domain.LocalisationDefaults{
		TimeZone: "Europe/Vienna", WeekStart: domain.WeekStartSaturday,
	}}
	defaults, _ := workspaceOutput(read, resolved, nil)["installation_defaults"].(usecase.Output)
	if defaults["time_zone"] != "Europe/Vienna" || defaults["week_start"] != "SATURDAY" {
		t.Errorf("the installation's defaults read %v", defaults)
	}
	if _, held := defaults["locale"]; held {
		t.Error("a language the installation did not decide is answered")
	}
	if _, held := workspaceOutput(read, nil, nil)["installation_defaults"]; held {
		t.Error("an installation without the level answers defaults")
	}
}
