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
	  "blocklist_file": "/etc/hubtask/passwords.txt"
	}`
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	body, found, err := Read(path)
	if err != nil || !found {
		t.Fatalf("a well-formed file answered %v (found %v)", err, found)
	}
	for _, area := range []string{"sign_in", "legal", "localisation", "quotas", "blocklist_file"} {
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
