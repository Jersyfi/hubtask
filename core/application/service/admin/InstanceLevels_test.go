// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The two areas SI-17 added to the instance level, and the one refusal the third door needs.
//
// The concept gives this milestone four areas — the sign-in switches, the legal links, the
// localisation defaults and the quota ceilings — and two of them are different in kind. A lock is
// meaningful on a ceiling and forbidden on a language, and the difference is a decision rather than
// an omission, so it is a test rather than a comment.

// "Eine Instanz gibt einen Standard, nie ein Schloss" (§5.7). Refused rather than ignored: a `PUT`
// that carried a lock and was partly obeyed is a `PUT` whose author believes something untrue about
// their own installation.
func TestALocalisationDefaultCannotBeLocked(t *testing.T) {
	level, err := InstanceLevelOf(map[string]any{
		"localisation": map[string]any{
			"locale": map[string]any{"value": "de", "locked": true},
		},
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a locked language answered %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "admin.localisation_never_locked" {
		t.Errorf("refused with %q", code)
	}
	if level.Localisation.Locale != "" {
		t.Error("the refused value was applied anyway")
	}

	// And without the lock it is an ordinary default.
	level, err = InstanceLevelOf(map[string]any{
		"localisation": map[string]any{
			"locale":     map[string]any{"value": "de"},
			"time_zone":  map[string]any{"value": "Europe/Berlin"},
			"week_start": map[string]any{"value": float64(1)},
		},
	})
	if err != nil {
		t.Fatalf("a plain default was refused: %v", err)
	}
	if level.Localisation.Locale != "de" || level.Localisation.TimeZone != "Europe/Berlin" {
		t.Errorf("the defaults read %+v", level.Localisation)
	}
	if level.Localisation.WeekStart != 1 {
		t.Errorf("the week start reads %d", level.Localisation.WeekStart)
	}
}

// A week start outside the seven days is refused rather than stored: the calendar would open on a
// day that does not exist, and nothing downstream would say why.
func TestAWeekStartOutsideTheWeekIsRefused(t *testing.T) {
	for _, value := range []float64{0, 8, -1} {
		_, err := InstanceLevelOf(map[string]any{
			"localisation": map[string]any{"week_start": map[string]any{"value": value}},
		})
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("a week start of %v answered %v", value, err)
		}
	}
}

// A ceiling *may* be locked, which is the difference from a language — and the difference is what
// "Tarif, Ausnahme je Bereich" will mean once there are plans.
func TestAQuotaCeilingCarriesItsLock(t *testing.T) {
	level, err := InstanceLevelOf(map[string]any{
		"quotas": map[string]any{
			quota.ExportJobs:     map[string]any{"value": float64(9), "locked": true},
			quota.AiTokensPerDay: map[string]any{"value": float64(50_000)},
		},
	})
	if err != nil {
		t.Fatalf("the ceilings were refused: %v", err)
	}
	if ceiling, decided := level.Quotas.Of(quota.ExportJobs); !decided || ceiling != 9 {
		t.Errorf("the export ceiling reads %d (decided %v)", ceiling, decided)
	}
	if !level.Quotas.Locked(quota.ExportJobs) {
		t.Error("the lock was dropped")
	}
	if level.Quotas.Locked(quota.AiTokensPerDay) {
		t.Error("a ceiling nobody locked came back locked")
	}

	// Zero is a decision — "unlimited" — and not an absence. A `!= 0` test somewhere would turn an
	// operator's deliberate "no ceiling" into "no opinion", which resolves to the opposite.
	unlimited, err := InstanceLevelOf(map[string]any{
		"quotas": map[string]any{quota.WebhookTargets: map[string]any{"value": float64(0)}},
	})
	if err != nil {
		t.Fatalf("an unlimited ceiling was refused: %v", err)
	}
	if ceiling, decided := unlimited.Quotas.Of(quota.WebhookTargets); !decided || ceiling != 0 {
		t.Errorf("a configured unlimited reads %d (decided %v)", ceiling, decided)
	}
}

// A ceiling that is not a whole number, or is negative, is refused: a limit of minus one is a limit
// nothing can be under.
func TestARefusedCeilingIsRefusedRatherThanRounded(t *testing.T) {
	for _, value := range []any{float64(-1), "many", float64(1.5)} {
		if _, err := InstanceLevelOf(map[string]any{
			"quotas": map[string]any{quota.Items: map[string]any{"value": value}},
		}); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("a ceiling of %v answered %v", value, err)
		}
	}
}

// A quota this build does not know is skipped rather than refused — the sign-in switches' rule, and
// for the same reason: a newer client should lose the key it has and this build does not, rather
// than lose the save.
func TestAnUnknownQuotaIsSkippedRatherThanRefused(t *testing.T) {
	level, err := InstanceLevelOf(map[string]any{
		"quotas": map[string]any{"teleportation_per_week": map[string]any{"value": float64(3)}},
	})
	if err != nil {
		t.Fatalf("an unknown quota was refused: %v", err)
	}
	if !level.Quotas.IsZero() {
		t.Errorf("an unknown quota was stored: %+v", level.Quotas.Limits)
	}
}

// A file in `enforce` mode is the source, and it is rewritten at every start. A save accepted here
// is a save the next restart silently undoes, which is worse than a refusal — the operator would
// believe their change took.
func TestAWriteIsRefusedWhileAFileEnforcesTheLevel(t *testing.T) {
	writer, settings, journal := newInstanceWriter(newRegister(operatorID))
	settings.level.Source = "/etc/hubtask/instance.json"
	settings.level.IsEnforcedFromFile = true

	_, err := (WriteInstanceSettings{Writer: writer}).
		Execute(t.Context(), operator(), identityrepo.InstanceLevel{})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a write under an enforcing file answered %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "admin.instance_enforced_from_file" {
		t.Errorf("refused with %q", code)
	}
	if shared.AsError(err).Params["source"] != "/etc/hubtask/instance.json" {
		t.Error("the refusal does not say which file is the source")
	}
	if len(settings.writes) != 0 {
		t.Error("the level was written despite the refusal")
	}
	if len(journal.entries) != 0 {
		t.Error("a refused write was journalled as one that happened")
	}
}

// And the file itself may write, because it *is* the source. It carries no actor: there is nobody
// to authorise at start-up, and the journal records where the values came from instead of a name.
func TestTheFileItselfMayWriteWhatTheAPIMayNot(t *testing.T) {
	writer, settings, journal := newInstanceWriter(newRegister(operatorID))

	level := identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{
			Patch: identity.PolicyPatch{MinLength: intOf(16)},
			Locks: map[identity.PolicySwitch]bool{identity.SwitchMinLength: true},
		},
		Legal: identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
	}
	if _, err := (WriteInstanceSettings{Writer: writer}).
		ExecuteAsFile(t.Context(), level, "/etc/hubtask/instance.json"); err != nil {
		t.Fatalf("the file's own write was refused: %v", err)
	}
	if len(settings.writes) != 1 {
		t.Fatalf("%d writes, want one", len(settings.writes))
	}
	if len(journal.entries) != 1 {
		t.Fatalf("%d journal entries, want one - a change from a file is as visible as a click", len(journal.entries))
	}
	if journal.entries[0].ActorLabel != "/etc/hubtask/instance.json" {
		t.Errorf("the journal names %q rather than the source", journal.entries[0].ActorLabel)
	}
}

// The projection answers the two new areas whole, for the switches' reason.
func TestTheProjectionAnswersTheLocalisationAndQuotaAreas(t *testing.T) {
	out := instanceLevelOutput(identityrepo.InstanceLevel{
		Policy:       identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:        identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Localisation: identity.LocalisationDefaults{Locale: "de"},
		Quotas: identity.QuotaDefaults{
			Limits: map[string]int64{quota.ExportJobs: 9},
			Locks:  map[string]bool{quota.ExportJobs: true},
		},
	})

	localisation, _ := out["localisation"].(usecase.Output)
	if len(localisation) != 3 {
		t.Errorf("the projection answers %d localisation defaults, want three", len(localisation))
	}
	locale, _ := localisation["locale"].(usecase.Output)
	if locale["set"] != true || locale["value"] != "de" {
		t.Errorf("the locale reads %v", locale)
	}
	zone, _ := localisation["time_zone"].(usecase.Output)
	if zone["set"] != false {
		t.Errorf("an undecided time zone reads %v", zone)
	}

	quotas, _ := out["quotas"].(usecase.Output)
	if len(quotas) != len(quota.Names()) {
		t.Errorf("the projection answers %d quotas, want every one of the %d", len(quotas), len(quota.Names()))
	}
	exports, _ := quotas[quota.ExportJobs].(usecase.Output)
	if exports["set"] != true || exports["value"] != int64(9) || exports["locked"] != true {
		t.Errorf("the export ceiling reads %v", exports)
	}
}
