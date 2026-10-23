// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package instancefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The third door, at the level this package is responsible for: a file becomes a request, or it is
// refused loudly. What the request then means is the use case's, which is the whole point of the
// package being this small.

func TestTheModeDefaultsToTheOneThatDoesLeast(t *testing.T) {
	for _, raw := range []string{"", "  ", "seed", "SEED"} {
		mode, err := ParseMode(raw)
		if err != nil || mode != ModeSeed {
			t.Errorf("%q read as %q (%v)", raw, mode, err)
		}
	}
	if mode, err := ParseMode("enforce"); err != nil || mode != ModeEnforce {
		t.Errorf("enforce read as %q (%v)", mode, err)
	}
	if _, err := ParseMode("overwrite"); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an invented mode answered %v", err)
	}
}

// A path that names nothing is not an error: the deployment this mode exists for is one where the
// file arrives with the next rollout, and refusing to start would make the mode useless there.
func TestAFileThatIsNotThereYetIsNotAnError(t *testing.T) {
	document, found, err := Read(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("an absent file answered %v", err)
	}
	if found || document != nil {
		t.Errorf("an absent file answered a document: %v", document)
	}
}

// A malformed file fails loudly, and this is the one place in the level's handling that does. A
// malformed row in the database is one setting nobody can read; a malformed file is an operator
// whose whole configuration silently did not apply.
func TestAMalformedFileIsRefusedRatherThanSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	_, _, err := Read(path)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a malformed file answered %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "config.instance_file_malformed" {
		t.Errorf("refused with %q", code)
	}
}

// An area nobody reads is refused for the reason a malformed file is: a file is read by every
// future start, so a typo that silently did nothing is a configuration somebody believes is in
// force. That is the opposite of the `PUT`'s rule, and deliberately — a request is made by
// somebody watching, a file is not.
func TestAnAreaThisBuildDoesNotKnowIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.json")
	if err := os.WriteFile(path, []byte(`{"sign_im": {}}`), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	_, _, err := Read(path)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a misspelled area answered %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "config.instance_file_area_unknown" {
		t.Errorf("refused with %q", code)
	}
	if shared.AsError(err).Params["area"] != "sign_im" {
		t.Error("the refusal does not say which name it did not know")
	}
}

// And a file the build does know becomes the request body the use case takes, unchanged. This
// package maps; it does not interpret.
func TestAKnownFileBecomesTheRequestBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.json")
	const document = `{
	  "sign_in": {"min_length": {"value": 16, "locked": true}},
	  "legal": {"imprint": {"value": "https://example.org/imprint"}},
	  "localisation": {"locale": {"value": "de"}},
	  "quotas": {"export_jobs": {"value": 9}},
	  "ai": {"sources": {"value": "OFFERED", "locked": true}},
	  "blocklist_file": "/etc/hubtask/passwords.txt",
	  "metering": true
	}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	body, found, err := Read(path)
	if err != nil || !found {
		t.Fatalf("a well-formed file answered %v (found %v)", err, found)
	}
	for _, area := range []string{"sign_in", "legal", "localisation", "quotas", "ai", "blocklist_file", "metering"} {
		if _, held := body[area]; !held {
			t.Errorf("%s did not survive the read", area)
		}
	}
	signIn, _ := body["sign_in"].(map[string]any)
	minLength, _ := signIn["min_length"].(map[string]any)
	if minLength["value"] != float64(16) || minLength["locked"] != true {
		t.Errorf("the switch reads %v", minLength)
	}
}

// The file names its offered AI models with the key's variable, never the key.
func TestTheFilesAiModelsTakeTheirKeyFromTheEnvironment(t *testing.T) {
	document := map[string]any{"ai_models": []any{
		map[string]any{"display_name": "Large (EU)", "processor": "A processor", "kind": "OPENAI_COMPATIBLE",
			"base_url": "https://offered.example/v1", "jurisdiction": "EEA", "api_key_env": "OFFERED_KEY"},
		map[string]any{"display_name": "Local", "processor": "Us", "kind": "OLLAMA",
			"base_url": "http://ollama:11434", "jurisdiction": "SELF_HOSTED"},
	}}
	lookup := func(name string) (string, bool) {
		if name == "OFFERED_KEY" {
			return "the-key", true
		}
		return "", false
	}
	models, present, err := AiModels(document, lookup, "instance.json")
	if err != nil || !present || len(models) != 2 {
		t.Fatalf("read %v %v %v", models, present, err)
	}
	if models[0].APIKey != "the-key" || !models[0].HasKey || models[1].HasKey {
		t.Errorf("keys %+v", models)
	}

	_, _, err = AiModels(document, func(string) (string, bool) { return "", false }, "instance.json")
	if got := shared.AsError(err); got == nil || got.DetailCode != "config.instance_file_ai_key_missing" {
		t.Errorf("a missing variable answered %v", err)
	}
	if _, present, _ := AiModels(map[string]any{}, lookup, "instance.json"); present {
		t.Error("a file without the key claims to name models")
	}
}

// Plans are rows of their own, each the document the API takes; a file without the key leaves
// plans to the other doors, and a plan without a name is a malformed file.
func TestPlansAreReadAsTheDocumentsTheAPITakes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.json")
	const document = `{"plans": [{"name": "family", "display_name": "Family",
	  "quotas": {"items": {"value": 5000, "locked": true}}, "left_out": ["WEBHOOKS"]}]}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	body, _, err := Read(path)
	if err != nil {
		t.Fatalf("a file with plans answered %v", err)
	}
	plans, present, err := Plans(body, path)
	if err != nil || !present || len(plans) != 1 || plans[0]["name"] != "family" {
		t.Fatalf("read %v (present %t, %v)", plans, present, err)
	}
	if _, present, _ := Plans(map[string]any{}, path); present {
		t.Error("a file without plans answered them present")
	}
	if _, _, err := Plans(map[string]any{"plans": []any{map[string]any{"display_name": "Nameless"}}}, path); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a plan without a name answered %v", err)
	}
}

// A subscription names its target and the file its secret is in - never the secret itself; a file
// without the key leaves the subscriptions to the other doors.
func TestPlatformSubscriptionsAreReadWithTheirSecretFromItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "platform.secret")
	if err := os.WriteFile(secretPath, []byte("  the-platform-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "instance.json")
	document := `{"platform_subscriptions": [{"target_url": "https://shop.example/hubtask", "secret_file": "` + secretPath + `"}]}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _, err := Read(path)
	if err != nil {
		t.Fatalf("a file with subscriptions answered %v", err)
	}
	subscriptions, present, err := PlatformSubscriptions(body, os.ReadFile, path)
	if err != nil || !present || len(subscriptions) != 1 ||
		subscriptions[0].TargetURL != "https://shop.example/hubtask" || subscriptions[0].Secret != "the-platform-secret" {
		t.Fatalf("read %+v (present %t, %v)", subscriptions, present, err)
	}
	if _, present, _ := PlatformSubscriptions(map[string]any{}, os.ReadFile, path); present {
		t.Error("a file without subscriptions answered them present")
	}
	for name, entry := range map[string]map[string]any{
		"no secret file": {"target_url": "https://shop.example"},
		"no target":      {"secret_file": secretPath},
	} {
		if _, _, err := PlatformSubscriptions(map[string]any{"platform_subscriptions": []any{entry}}, os.ReadFile, path); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
	missing := map[string]any{"platform_subscriptions": []any{map[string]any{
		"target_url": "https://shop.example", "secret_file": filepath.Join(dir, "absent"),
	}}}
	if _, _, err := PlatformSubscriptions(missing, os.ReadFile, path); err == nil {
		t.Error("a secret file that is not there was read")
	}
}

// The installation's presets (UC-INS-18 check 1): each the document the API takes, known by its
// name, so a name twice is refused rather than one of the two silently winning.
func TestPresetsAreReadAsTheDocumentsTheAPITakes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.json")
	const document = `{"presets": [{"name": "Craft order", "preset": "FLOW", "is_default": true,
	  "locks": ["views"], "settings": {"levels": [{"type": "TASK", "name": "Order"}]}}]}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	body, _, err := Read(path)
	if err != nil {
		t.Fatalf("a file with presets answered %v", err)
	}
	presets, present, err := Presets(body, path)
	if err != nil || !present || len(presets) != 1 || presets[0]["name"] != "Craft order" || presets[0]["is_default"] != true {
		t.Fatalf("read %v (present %t, %v)", presets, present, err)
	}
	if _, present, _ := Presets(map[string]any{}, path); present {
		t.Error("a file without presets answered them present")
	}
	if presets, present, err := Presets(map[string]any{"presets": []any{}}, path); err != nil || !present || len(presets) != 0 {
		t.Errorf("an empty list read as %v (present %t, %v)", presets, present, err)
	}
	twice := map[string]any{"presets": []any{map[string]any{"name": "A"}, map[string]any{"name": "A"}}}
	if _, _, err := Presets(twice, path); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a name twice answered %v", err)
	}
	if _, _, err := Presets(map[string]any{"presets": []any{map[string]any{"preset": "LIST"}}}, path); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a preset without a name answered %v", err)
	}
}
