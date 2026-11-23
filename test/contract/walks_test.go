// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	walkBaselinePath = "testdata/walks-baseline.txt"
	webappPath       = "../../apps/webapp"
)

func health(status int, body string) walkAnswer {
	contentType := "application/json"
	return walkAnswer{Method: "GET", Path: "/meta/health", Status: status, ContentType: &contentType, Body: &body}
}

// The judge is shown answers it must refuse before it is trusted with the walks' own.
func TestTheWalkJudgeNamesEachWayOutOfTheContract(t *testing.T) {
	spec := contractSpec(t)
	operations, err := spec.walkOperations()
	if err != nil {
		t.Fatalf("%v", err)
	}
	full := `{"status":"ok","version":"0.1.0","dependencies":[]}`
	elsewhere := health(200, full)
	elsewhere.Path = "/meta/horoscope/01a0e2e0-0000-7000-8000-000000000001/7"
	stream, lines, plainJSON, png := "text/event-stream", "application/x-ndjson; charset=utf-8", "application/json", "image/png"
	picture := "/auth/workspace-pictures/01a0e2e0-0000-7000-8000-000000000001"

	cases := []struct {
		name   string
		answer walkAnswer
		want   string
	}{
		{"an answer the contract allows", health(200, full), ""},
		{"an operation the contract does not have", elsewhere, "unknown-operation GET /meta/horoscope/{id}/{n} 200"},
		{"a status the operation does not declare", health(302, full), "status-undeclared GET /meta/health 302"},
		{"a body outside the schema", health(200, `{"status":"ok"}`), "body-mismatch GET /meta/health 200"},
		{"no body where one is declared", health(200, ""), "body-mismatch GET /meta/health 200"},
		{"a body that is not JSON", health(200, "<html>"), "body-mismatch GET /meta/health 200"},
		{"a second value after the body", health(200, full+` {"status":"wrong"}`), "body-mismatch GET /meta/health 200"},
		{"text after the body", health(200, full+" and more"), "body-mismatch GET /meta/health 200"},
		{"a line's end after the body", health(200, full+"\n"), ""},
		{"a refusal that is no problem", health(403, `{"oops":true}`), "body-mismatch GET /meta/health 403"},
		{"a body the record does not hold", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, BodyFrom: "path"}, ""},
		{"a media type the response does not declare", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, ContentType: &stream}, "content-type-undeclared GET /meta/health 200"},
		{"JSON where the response declares none", walkAnswer{Method: "POST", Path: "/sync:snapshot", Status: 200, ContentType: &plainJSON, Body: &full}, "content-type-undeclared POST /sync:snapshot 200"},
		{"a problem as plain JSON is judged as the problem", health(403, `{"oops":true}`), "body-mismatch GET /meta/health 403"},
		{"a declared media type that is not JSON", walkAnswer{Method: "POST", Path: "/sync:snapshot", Status: 200, ContentType: &lines, Body: &full}, ""},
		{"no media type at all", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, Body: &full}, ""},
		// A response declaring a range (`image/*`) admits each type inside it, as OpenAPI says - and no other.
		{"a type inside a declared range", walkAnswer{Method: "GET", Path: picture, Status: 200, ContentType: &png}, ""},
		{"a type outside a declared range", walkAnswer{Method: "GET", Path: picture, Status: 200, ContentType: &stream}, "content-type-undeclared GET /auth/workspace-pictures/{mediaId} 200"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			if violation := spec.judgeWalkAnswer(operations, tc.answer); violation != nil {
				got = violation.key()
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `/accounts/me` is an operation of its own and not an account called "me", and a custom verb
// belongs to its own operation rather than to the resource before the colon.
func TestTheWalkJudgeFindsTheMostLiteralOperation(t *testing.T) {
	spec := contractSpec(t)
	operations, err := spec.walkOperations()
	if err != nil {
		t.Fatalf("%v", err)
	}
	for path, want := range map[string]string{
		"/accounts/me": "GET /accounts/me",
		"/accounts/01a0e2e0-0000-7000-8000-000000000001/stars": "GET /accounts/{accountId}/stars",
	} {
		violation := spec.judgeWalkAnswer(operations, walkAnswer{Method: "GET", Path: path, Status: 399})
		if violation == nil || violation.Operation != want {
			t.Errorf("%s: judged as %+v, want the operation %s", path, violation, want)
		}
	}
}

// One wrong answer is one violation however many suites give it, the fixture's guess is none, and
// the baseline is compared by kind, operation and status alone.
func TestAViolationIsKnownByKindOperationAndStatus(t *testing.T) {
	short := `{"status":"ok"}`
	guess := health(404, short)
	guess.Guess = true
	found, err := contractSpec(t).judgeWalkRecords(map[string][]walkAnswer{
		"b": {health(200, short), health(302, short)},
		"a": {health(200, short), guess},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	body := found["body-mismatch GET /meta/health 200"]
	if len(found) != 2 || !slices.Equal(body.Suites, []string{"a", "b"}) {
		t.Fatalf("found %+v, want two violations, the body's in both suites", found)
	}
	unknown, notMet := againstBaseline(found, []string{"body-mismatch GET /meta/health 200", "body-mismatch GET /tenant 200"})
	if len(unknown) != 1 || unknown[0].key() != "status-undeclared GET /meta/health 302" {
		t.Errorf("unknown = %+v, want the one violation the baseline does not hold", unknown)
	}
	if !slices.Equal(notMet, []string{"body-mismatch GET /tenant 200"}) {
		t.Errorf("notMet = %v, want the entry no record met", notMet)
	}
}

// Pruning removes what no record met, so it needs a record of every suite `make e2e` walks: after
// a run of one suite it would remove nearly the whole baseline.
func TestPruningNeedsARecordOfEverySuite(t *testing.T) {
	suites, err := walkedSuites(webappPath)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(suites) < 100 || !slices.Contains(suites, "terms") {
		t.Fatalf("found %d suites and not `terms` - the list misses the walks", len(suites))
	}
	// Left out by the web app's `test:engines` script, so a full run leaves no record of it.
	if slices.Contains(suites, "whiteboardframes") {
		t.Errorf("whiteboardframes is listed, and `make e2e` does not walk it")
	}
	// A test of a helper: it opens no browser, so the recorder is never loaded.
	if slices.Contains(suites, "scalewalk-errors") {
		t.Errorf("scalewalk-errors is listed, and it leaves no record")
	}
	records := map[string][]walkAnswer{}
	for _, suite := range suites {
		records[suite] = []walkAnswer{}
	}
	if missing := withoutRecord(suites, records); len(missing) != 0 {
		t.Errorf("every suite has a record, and %v are named as missing", missing)
	}
	delete(records, "terms")
	if missing := withoutRecord(suites, records); !slices.Equal(missing, []string{"terms"}) {
		t.Errorf("missing = %v, want the one suite without a record", missing)
	}
}

func TestTheSuitesAScriptLeavesOut(t *testing.T) {
	for script, want := range map[string][]string{
		`node --test "e2e/!(whiteboardframes).test.mjs"`: {"whiteboardframes"},
		`node --test "e2e/!(a|b-c).test.mjs"`:            {"a", "b-c"},
		`node --test "e2e/*.test.mjs"`:                   nil,
	} {
		if got := leftOutSuites(script); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", script, got, want)
		}
	}
}

// An entry of the baseline excuses every wrong body of its operation and status, and the gate says
// how much that is.
func TestTheBaselinesCoarsenessIsMeasured(t *testing.T) {
	spec := contractSpec(t)
	operations, err := spec.walkOperations()
	if err != nil {
		t.Fatalf("%v", err)
	}
	guess := health(200, "{}")
	guess.Guess = true
	elsewhere := health(200, "{}")
	elsewhere.Path = "/meta/horoscope"
	records := map[string][]walkAnswer{
		"a": {health(200, "{}"), health(403, "{}"), guess},
		"b": {health(200, `{"other":1}`), elsewhere},
	}
	baseline := []string{"body-mismatch GET /meta/health 200", "body-mismatch GET /meta/health 404", "status-undeclared GET /tenant 302"}
	excused, onExcusedKey, judged := baselineCoarseness(operations, records, baseline)
	if excused != 1 || onExcusedKey != 2 || judged != 4 {
		t.Fatalf("got %d operations, %d of %d answers; want 1 operation, 2 of 4 answers", excused, onExcusedKey, judged)
	}
}

// The walks' recorded answers against the contract (ci-cd.md §3, `engines`). WALK_RECORD names
// the directory the walks wrote to; without it there is nothing to judge. A violation the baseline
// does not hold fails. WALK_BASELINE=prune removes the entries that were not met, and only where
// every suite has a record - nothing here ever adds one.
func TestTheWalksAnswerWithinTheContract(t *testing.T) {
	directory := os.Getenv("WALK_RECORD")
	if directory == "" {
		t.Skip("WALK_RECORD names no directory of recorded walks")
	}
	records, err := readWalkRecords(directory)
	if err != nil {
		t.Fatalf("reading the records: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("%s holds no record: the walks did not run with WALK_RECORD set", directory)
	}
	spec := contractSpec(t)
	found, err := spec.judgeWalkRecords(records)
	if err != nil {
		t.Fatalf("%v", err)
	}
	baseline, err := readWalkBaseline(walkBaselinePath)
	if err != nil {
		t.Fatalf("reading the baseline: %v", err)
	}
	if !sort.StringsAreSorted(baseline) {
		t.Errorf("%s is not sorted", walkBaselinePath)
	}

	answers, guesses, lines := 0, 0, 0
	for _, suite := range records {
		answers += len(suite)
		for _, answer := range suite {
			if answer.Guess {
				guesses++
			} else if mediaTypeOf(answer.ContentType) == "application/x-ndjson" {
				lines++
			}
		}
	}
	kinds := map[string]int{}
	for _, violation := range found {
		kinds[violation.Kind]++
	}
	t.Logf("%d suites, %d distinct answers; not judged: %d guesses of the fixture, the bodies of %d answers in application/x-ndjson; outside the contract, by operation and status: %d unknown operations, %d undeclared statuses, %d undeclared content types, %d bodies",
		len(records), answers, guesses, lines, kinds[unknownOperation], kinds[statusUndeclared], kinds[contentTypeUndeclared], kinds[bodyMismatch])
	operations, err := spec.walkOperations()
	if err != nil {
		t.Fatalf("%v", err)
	}
	excused, onExcusedKey, judged := baselineCoarseness(operations, records, baseline)
	t.Logf("the baseline lists a wrong body for %d of the contract's %d operations; %d of the %d judged answers are for an operation and status it lists, where another wrong body passes",
		excused, len(operations), onExcusedKey, judged)

	unknown, notMet := againstBaseline(found, baseline)
	for _, violation := range unknown {
		t.Errorf("%s, in %s: %s", violation.key(), strings.Join(violation.Suites, ", "), violation.Detail)
	}
	if len(unknown) > 0 {
		t.Errorf("%d answers leave the contract and are not in %s: answer as api/openapi.yaml declares", len(unknown), walkBaselinePath)
	}
	if os.Getenv("WALK_BASELINE") != "prune" || len(notMet) == 0 {
		return
	}
	suites, err := walkedSuites(webappPath)
	if err != nil {
		t.Fatalf("listing the suites: %v", err)
	}
	if missing := withoutRecord(suites, records); len(missing) > 0 {
		t.Fatalf("not pruned: %d of %d suites have no record in %s, so an entry only they meet would be removed - walk every suite with 'make e2e RECORD=1' first. Missing: %s",
			len(missing), len(suites), directory, strings.Join(missing, ", "))
	}
	if err := pruneWalkBaseline(walkBaselinePath, notMet); err != nil {
		t.Fatalf("pruning the baseline: %v", err)
	}
	t.Logf("removed %d entries from %s", len(notMet), walkBaselinePath)
}

// pruneWalkBaseline rewrites the baseline without the given entries, comments kept.
func pruneWalkBaseline(path string, gone []string) error {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if !slices.Contains(gone, strings.TrimSpace(line)) {
			kept = append(kept, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}
