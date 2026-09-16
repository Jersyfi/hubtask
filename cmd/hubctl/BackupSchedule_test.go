// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The trial restore flag (B-4, P-14): shown by the listing, and moved by `set --trial` and
// `--no-trial` - and by nothing else, so that `--off` leaves it alone.
func TestTheScheduleListingShowsTheTrialAndSetMovesIt(t *testing.T) {
	schedule := `{"id":"` + itemID + `","target_id":"` + targetID + `","scope":{"kind":"TENANT"},` +
		`"rrule":"FREQ=DAILY;BYHOUR=3","timezone":"UTC","mode":"FULL","enabled":true,"trial_restore":true}`
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("[" + schedule + "]"))
			return
		}
		_, _ = w.Write([]byte(strings.Replace(schedule, `"trial_restore":true`, `"trial_restore":false`, 1)))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "backup", "schedule", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "TRIAL") || !strings.Contains(out, "yes") {
		t.Errorf("the listing does not show the trial: %s", out)
	}

	code, out, errOut = invokeAgainst(t, stub, signedIn(stub), "", "backup", "schedule", "set", itemID, "--no-trial")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(stub.body), &sent)
	if sent["trial_restore"] != false || len(sent) != 1 {
		t.Errorf("the patch carries %v, want the flag alone", sent)
	}
	if !strings.Contains(out, "no") {
		t.Errorf("the answer does not show the flag off: %s", out)
	}

	// --off moves the state and nothing else.
	if code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "backup", "schedule", "set", itemID, "--off"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	sent = nil
	_ = json.Unmarshal([]byte(stub.body), &sent)
	if _, moved := sent["trial_restore"]; moved || sent["enabled"] != false {
		t.Errorf("--off sent %v", sent)
	}
	if code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "backup", "schedule", "set", itemID, "--trial", "--no-trial"); code == exitOK {
		t.Errorf("--trial with --no-trial was accepted: %s", errOut)
	}
}
