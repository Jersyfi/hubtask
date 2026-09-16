// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const deviceID = "01936f2a-7c1e-7000-8000-0000000000e1"

// A pull pages until there is no more and prints the records as JSON lines with the cursor
// last; the device is the profile's, minted on first use and kept.
func TestPullingWritesTheRecordsAsLinesAndTheCursorLast(t *testing.T) {
	var requests []map[string]any
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		var request map[string]any
		_ = json.Unmarshal([]byte(stub.body), &request)
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"changes":[{"seq":1,"entity":"item","entity_id":"` + itemID + `","op":"UPSERT"}],"cursor":"c1","has_more":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"changes":[{"seq":2,"entity":"item","entity_id":"` + itemID + `","op":"DELETE"}],"cursor":"c2","has_more":false}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	env := signedIn(stub)
	env[envProfile] = profile

	code, out, errOut := invokeAgainst(t, stub, env, "", "sync", "pull", "--all", "--scope", collectionID+":SELF", "--limit", "2")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], `"UPSERT"`) || !strings.Contains(lines[1], `"DELETE"`) {
		t.Fatalf("the output is %q, want two records and the cursor", out)
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil || last["cursor"] != "c2" {
		t.Errorf("the last line is %q, want the cursor", lines[2])
	}
	if len(requests) != 2 || requests[1]["cursor"] != "c1" {
		t.Errorf("the second page did not continue from the first's cursor: %v", requests)
	}
	if scopes, _ := requests[0]["scopes"].([]any); len(scopes) != 1 {
		t.Errorf("the scope was not sent: %v", requests[0])
	}
	if requests[0]["limit"] != float64(2) || requests[0]["platform"] != syncPlatform {
		t.Errorf("the request carries %v", requests[0])
	}

	// The device was minted, kept, and sent both times.
	stored, err := LoadProfile(profile)
	if err != nil || stored.Device == "" {
		t.Fatalf("the profile holds no device (%v)", err)
	}
	if requests[0]["device_id"] != stored.Device || requests[1]["device_id"] != stored.Device {
		t.Errorf("the pull did not act as the profile's device %s: %v", stored.Device, requests)
	}
	if !strings.Contains(errOut, stored.Device) {
		t.Errorf("the minting was not announced on standard error: %q", errOut)
	}
}

// A push sends the file's lines as they were, stamps a reading where a line carries none from a
// clock that ticks on across invocations and can be skewed, and prints one result per line.
func TestPushingStampsReadingsAndPrintsOneResultPerLine(t *testing.T) {
	var request map[string]any
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.Unmarshal([]byte(stub.body), &request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"op_id":"01936f2a-7c1e-7000-8000-0000000000a1","result":"APPLIED"},{"op_id":"01936f2a-7c1e-7000-8000-0000000000a2","result":"REJECTED","error":{"code":"forbidden","message_code":"access.not_permitted"}}],"cursor":"c9"}`))
	})
	dir := t.TempDir()
	file := filepath.Join(dir, "mutations.jsonl")
	stamped, _ := shared.NewHLC(time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC), 3, deviceID)
	if err := os.WriteFile(file, []byte(strings.Join([]string{
		`{"op_id":"01936f2a-7c1e-7000-8000-0000000000a1","kind":"ITEM_CREATE","item_id":"` + itemID + `","payload":{"type":"TASK","title":"Milk","extra":"kept"}}`,
		"",
		`# a comment`,
		`{"op_id":"01936f2a-7c1e-7000-8000-0000000000a2","kind":"ITEM_PATCH","item_id":"` + itemID + `","fields":{"title":{"value":"Oat milk"},"notes":{"value":"x","hlc":"` + stamped.String() + `"}}}`,
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	env := signedIn(stub)
	env[envProfile] = filepath.Join(dir, "profile.json")

	code, out, errOut := invokeAgainst(t, stub, env, "", "sync", "push", "--file", file, "--device", deviceID, "--clock-offset", "3h")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "APPLIED") || !strings.Contains(lines[1], "access.not_permitted") {
		t.Errorf("the output is %q, want one result per line", out)
	}
	mutations, _ := request["mutations"].([]any)
	if len(mutations) != 2 || request["device_id"] != deviceID {
		t.Fatalf("the request carries %v", request)
	}
	first, _ := mutations[0].(map[string]any)
	payload, _ := first["payload"].(map[string]any)
	if payload["extra"] != "kept" {
		t.Errorf("the line did not round-trip unchanged: %v", first)
	}
	reading, err := shared.ParseHLC(first["hlc"].(string))
	if err != nil {
		t.Fatalf("the creation was stamped %v: %v", first["hlc"], err)
	}
	if drift := time.Until(reading.Physical); drift < 2*time.Hour+50*time.Minute || drift > 3*time.Hour+time.Minute {
		t.Errorf("the reading is %v from now, want the three hours of the offset", drift)
	}
	if reading.Device != deviceID {
		t.Errorf("the reading names device %q", reading.Device)
	}
	second, _ := mutations[1].(map[string]any)
	fields, _ := second["fields"].(map[string]any)
	title, _ := fields["title"].(map[string]any)
	notes, _ := fields["notes"].(map[string]any)
	if title["hlc"] == nil || notes["hlc"] != stamped.String() {
		t.Errorf("the fields carry %v and %v, want the title stamped and the notes as written", title, notes)
	}
	if _, stampedWhole := second["hlc"]; stampedWhole {
		t.Errorf("a patch with fields was stamped as a whole: %v", second)
	}

	// The clock is kept, and the next stamping ticks on from it rather than starting over.
	stored, err := LoadProfile(env[envProfile])
	if err != nil || stored.Clock == "" {
		t.Fatalf("the profile holds no clock (%v)", err)
	}
	kept, _ := shared.ParseHLC(stored.Clock)
	if kept.Compare(reading) < 0 {
		t.Errorf("the profile kept %s, before the last reading %s", stored.Clock, reading)
	}
}

func TestPushingRefusesAFileWithoutOperationIdentifiers(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	file := filepath.Join(t.TempDir(), "mutations.jsonl")
	if err := os.WriteFile(file, []byte(`{"kind":"ITEM_CREATE"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "sync", "push", "--file", file, "--device", deviceID)
	if code != exitUsage || !strings.Contains(errOut, "op_id") {
		t.Errorf("exit %d, %q; want a usage error naming op_id", code, errOut)
	}
}

func TestDevicesAreListedAndForgotten(t *testing.T) {
	var forgotten string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			forgotten = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"` + deviceID + `","platform":"hubctl","display_name":"shell","blocked":false,"last_seen_at":"2026-09-16T08:00:00Z"}]`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveProfile(profile, Profile{Device: deviceID, Clock: "1:1:" + deviceID}); err != nil {
		t.Fatal(err)
	}
	env := signedIn(stub)
	env[envProfile] = profile

	code, out, errOut := invokeAgainst(t, stub, env, "", "sync", "devices", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, deviceID) || !strings.Contains(out, "hubctl") {
		t.Errorf("the listing is %q", out)
	}

	code, _, errOut = invokeAgainst(t, stub, env, "", "sync", "devices", "forget", deviceID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if forgotten != APIPath+syncDevicesPath+"/"+deviceID {
		t.Errorf("the forgetting called %q", forgotten)
	}
	// Forgetting the shell's own device clears it from the profile, so that the next pull mints
	// a new one rather than pushing as a device the server refuses.
	stored, _ := LoadProfile(profile)
	if stored.Device != "" || stored.Clock != "" {
		t.Errorf("the profile still holds device %q and clock %q", stored.Device, stored.Clock)
	}
}
