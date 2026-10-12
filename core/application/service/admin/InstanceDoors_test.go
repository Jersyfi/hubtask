// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The three doors refuse the same values (UC-INS-09 check 6).
//
// The screen and `hubctl` reach the level through the registry, as REST hands it the decoded body;
// the file reaches it through `InstanceLevelOf`. One table drives both, and each row has to come
// back with the same code at the same field - or, for an accepted row, with the same stored value.

// doorRow is one value sent to one key, and what every door must answer.
type doorRow struct {
	area, key string
	value     any
	// code is the refusal's detail code, empty for a value every door accepts.
	code string
	// path is the field the refusal names.
	path string
}

func doorRows() []doorRow {
	return []doorRow{
		{area: "sign_in", key: "min_length", value: float64(12)},
		{area: "sign_in", key: "min_length", value: "12", code: "auth.policy_value_invalid", path: "/sign_in/min_length"},
		{area: "sign_in", key: "min_length", value: float64(3.5), code: "auth.policy_value_invalid", path: "/sign_in/min_length"},
		{area: "sign_in", key: "max_repeat", value: float64(-1), code: "auth.policy_value_invalid", path: "/sign_in/max_repeat"},
		{area: "sign_in", key: "breach_check", value: "true", code: "auth.policy_value_invalid", path: "/sign_in/breach_check"},
		{area: "sign_in", key: "mfa_required_for", value: "SOMETIMES", code: "auth.policy_value_invalid", path: "/sign_in/mfa_required_for"},
		{area: "sign_in", key: "methods", value: []any{"PASSWORD", "CARRIER_PIGEON"}, code: "auth.policy_value_invalid", path: "/sign_in/methods"},
		{area: "sign_in", key: "methods", value: []any{"PASSWORD"}},
		{area: "legal", key: "accessibility_url", value: "https://example.org/a11y"},
		{area: "legal", key: "accessibility_url", value: "ftp://example.org/a11y", code: "auth.legal_url_scheme", path: "/legal/accessibility_url"},
		{area: "localisation", key: "locale", value: "de"},
		{area: "localisation", key: "locale", value: "not a language", code: "admin.locale_invalid", path: "/localisation/locale"},
		{area: "localisation", key: "time_zone", value: "Mars/Olympus_Mons", code: "admin.time_zone_invalid", path: "/localisation/time_zone"},
		{area: "localisation", key: "week_start", value: "SUNDAY"},
		{area: "localisation", key: "week_start", value: float64(4), code: "auth.policy_value_invalid", path: "/localisation/week_start"},
		{area: "quotas", key: quota.Items, value: float64(500)},
		{area: "quotas", key: quota.Items, value: "lots", code: "auth.policy_value_invalid", path: "/quotas/items"},
		{area: "ai", key: "sources", value: "OFFERED"},
		{area: "ai", key: "sources", value: "SOMEBODY", code: "auth.policy_value_invalid", path: "/ai/sources"},
	}
}

func TestTheScreenHubctlAndTheFileRefuseTheSameValues(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	registry, err := usecase.NewRegistry(nil, WriteInstanceSettings{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	for _, row := range doorRows() {
		document := map[string]any{row.area: map[string]any{row.key: map[string]any{"value": row.value, "locked": false}}}

		// The REST door, as the registry receives it: a JSON body decoded into the input map.
		var decoded map[string]any
		encoded, _ := json.Marshal(document)
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		_, throughRegistry := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input(decoded))
		// The file's door, with the document as a file would carry it.
		fromFile, throughFile := InstanceLevelOf(document)

		for door, err := range map[string]error{"registry": throughRegistry, "file": throughFile} {
			if row.code == "" {
				if err != nil {
					t.Errorf("%s.%s = %v through the %s: refused with %v", row.area, row.key, row.value, door, err)
				}
				continue
			}
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != row.code {
				t.Errorf("%s.%s = %v through the %s: %v, want %s", row.area, row.key, row.value, door, err, row.code)
				continue
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != row.path {
				t.Errorf("%s.%s through the %s names %v, want %s", row.area, row.key, door, refusal.Fields, row.path)
			}
		}
		if row.code == "" && isEmptyLevel(fromFile) {
			t.Errorf("%s.%s = %v from the file stored nothing", row.area, row.key, row.value)
		}
	}
}

// An entry without a value is a key the installation does not decide - absent or `null` in every
// area alike, and an empty word for the kinds that are words - whichever door it came through. A
// screen or a terminal sends the whole level back, undecided keys included.
func TestAnEntryWithoutAValueIsUndecidedInEveryArea(t *testing.T) {
	everyArea := map[string][]string{
		"sign_in":      {"min_length", "breach_check", "methods"},
		"legal":        {"imprint_url"},
		"localisation": {"locale", "time_zone", "week_start"},
		"quotas":       {quota.Items},
		"ai":           {"sources"},
	}
	words := map[string][]string{
		"legal":        {"imprint_url"},
		"localisation": {"locale", "time_zone"},
	}
	for _, sent := range []struct {
		entry map[string]any
		keys  map[string][]string
	}{
		{map[string]any{"set": false, "locked": false}, everyArea},
		{map[string]any{"value": nil, "locked": true}, everyArea},
		{map[string]any{"value": "  "}, words},
	} {
		entry := sent.entry
		document := map[string]any{}
		for area, keys := range sent.keys {
			sent := map[string]any{}
			for _, key := range keys {
				sent[key] = entry
			}
			document[area] = sent
		}
		level, err := InstanceLevelOf(document)
		if err != nil {
			t.Errorf("%v answered %v", entry, err)
			continue
		}
		if !isEmptyLevel(level) {
			t.Errorf("%v decided something: %+v", entry, level)
		}
	}
}

// Every key the level answers says its kind, and a choice its closed set: a
// key added without one is a key a screen cannot draw.
func TestEveryKeyOfEveryAreaIsDeclared(t *testing.T) {
	out := instanceLevelOutput(identityrepo.InstanceLevel{})
	for _, area := range []string{areaSignIn, areaLegal, areaLocalisation, areaQuotas, areaAi} {
		entries, _ := out[area].(usecase.Output)
		if len(entries) == 0 {
			t.Errorf("%s answers no keys", area)
		}
		places := map[any]string{}
		for name, raw := range entries {
			entry, _ := raw.(usecase.Output)
			if position, _ := entry["position"].(int); position < 0 || position >= len(entries) || places[position] != "" {
				t.Errorf("%s.%s is at %v, which is no place of its own", area, name, entry["position"])
			}
			places[entry["position"]] = name
			kind, _ := entry["kind"].(string)
			if !slices.Contains([]string{"COUNT", "FLAG", "CHOICE", "CHOICES", "LINK", "LOCALE", "TIME_ZONE"}, kind) {
				t.Errorf("%s.%s has no kind: %v", area, name, entry)
			}
			if options, _ := entry["options"].([]any); (kind == "CHOICE" || kind == "CHOICES") && len(options) == 0 {
				t.Errorf("%s.%s is a choice without options", area, name)
			}
			_, mayLock := entry["workspace_may"]
			if mayLock == (area == areaLocalisation) {
				t.Errorf("%s.%s: workspace_may present %v", area, name, mayLock)
			}
		}
	}
	for _, name := range integration.AiSwitches() {
		if _, held := integration.DeclarationOfAiSwitch(name); !held {
			t.Errorf("ai.%s has no declaration", name)
		}
	}
	if _, held := identity.DeclarationOfSwitch(identity.SwitchRotationFrom); held {
		t.Error("rotation_from is declared, though the installation does not hold it")
	}
}

// isEmptyLevel reports whether a level decides nothing and locks nothing.
func isEmptyLevel(level identityrepo.InstanceLevel) bool {
	return level.Policy.Patch.IsEmpty() && len(level.Policy.Locks) == 0 &&
		level.Legal.Links.IsEmpty() && len(level.Legal.Locks) == 0 &&
		level.Localisation.IsZero() && level.Quotas.IsZero() && len(level.Quotas.Locks) == 0 &&
		level.Ai.IsZero() && len(level.Ai.Locks) == 0
}

// A refused value never reaches the store, whichever door it came through.
func TestARefusedValueIsNotWritten(t *testing.T) {
	writer, store, _ := newInstanceWriter(newRegister(operatorID))
	_, err := WriteInstanceSettings{Writer: writer}.Execute(t.Context(), operator(), identityrepo.InstanceLevel{})
	if err != nil {
		t.Fatalf("writing an empty level: %v", err)
	}
	if _, err := InstanceLevelOf(map[string]any{
		"quotas": map[string]any{quota.Items: map[string]any{"value": float64(-3)}},
	}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a negative ceiling answered %v", err)
	}
	if len(store.writes) != 1 || !isEmptyLevel(store.level) {
		t.Errorf("the store holds %+v", store.level)
	}
}
