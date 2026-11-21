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

const walkBaselinePath = "testdata/walks-baseline.txt"

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
	stream, lines, plainJSON := "text/event-stream", "application/x-ndjson; charset=utf-8", "application/json"

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
		{"a refusal that is no problem", health(403, `{"oops":true}`), "body-mismatch GET /meta/health 403"},
		{"a body the record does not hold", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, BodyFrom: "path"}, ""},
		{"a media type the response does not declare", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, ContentType: &stream}, "content-type-undeclared GET /meta/health 200"},
		{"JSON where the response declares none", walkAnswer{Method: "POST", Path: "/sync:snapshot", Status: 200, ContentType: &plainJSON, Body: &full}, "content-type-undeclared POST /sync:snapshot 200"},
		{"a problem as plain JSON is judged as the problem", health(403, `{"oops":true}`), "body-mismatch GET /meta/health 403"},
		{"a declared media type that is not JSON", walkAnswer{Method: "POST", Path: "/sync:snapshot", Status: 200, ContentType: &lines, Body: &full}, ""},
		{"no media type at all", walkAnswer{Method: "GET", Path: "/meta/health", Status: 200, Body: &full}, ""},
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

// The walks' recorded answers against the contract (ci-cd.md §3, `engines`). WALK_RECORD names
// the directory the walks wrote to; without it there is nothing to judge. A violation the baseline
// does not hold fails. WALK_BASELINE=prune, after a run of every suite, removes the entries that
// were not met - nothing here ever adds one.
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
	found, err := contractSpec(t).judgeWalkRecords(records)
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
