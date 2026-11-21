// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The browser walks answer every request themselves (ci-cd.md §3, `engines`), so a walk proves
// a screen against what a worker typed. This file holds those answers to the contract: the walks
// record them (e2e/record.mjs), and each is judged as a server's response would be.

// walkAnswer is one line of a suite's record.
type walkAnswer struct {
	Method      string  `json:"method"`
	Path        string  `json:"path"`
	Status      int     `json:"status"`
	ContentType *string `json:"content_type"`
	Body        *string `json:"body"`
	// BodyBase64 and BodyFrom mark a body the record does not hold as text; it is not judged.
	BodyBase64 string `json:"body_base64"`
	BodyFrom   string `json:"body_from"`
	// Guess marks the fixture's answer to a request nothing stubbed: one empty page for whatever
	// was asked. It is counted and not judged - nobody wrote it for the operation, and which
	// reads a walk leaves unanswered until it ends differs from run to run.
	Guess bool `json:"guess"`
}

// The four ways an answer leaves the contract.
const (
	unknownOperation      = "unknown-operation"
	statusUndeclared      = "status-undeclared"
	contentTypeUndeclared = "content-type-undeclared"
	bodyMismatch          = "body-mismatch"
)

// walkViolation is one way the walks' answers leave the contract. Its key names the kind, the
// operation and the status. Not the body, which differs from run to run - and not the suite: which
// reads a walk still gets answered before it ends depends on timing, so the same wrong answer of
// the fixture shows in one suite in one run and in another in the next.
type walkViolation struct {
	Kind      string
	Operation string
	Status    int
	// Suites are the suites that gave the answer, and Detail what the first of them got wrong.
	Suites []string
	Detail string
}

func (v walkViolation) key() string {
	return fmt.Sprintf("%s %s %d", v.Kind, v.Operation, v.Status)
}

// walkOperation is one operation of the contract, findable by a concrete path.
type walkOperation struct {
	route    string
	method   string
	pattern  *regexp.Regexp
	literals int
	op       *operation
}

var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// walkOperations lists the contract's operations for matching. A parameter stands for one path
// segment or part of one (`/containers/{id}:archive`).
func (s *specification) walkOperations() ([]walkOperation, error) {
	routes, err := s.Routes("")
	if err != nil {
		return nil, err
	}
	operations := make([]walkOperation, 0, len(routes))
	for route, op := range routes {
		method, path, _ := strings.Cut(route, " ")
		var pattern strings.Builder
		pattern.WriteString("^")
		rest := path
		for _, at := range pathParameter.FindAllStringIndex(path, -1) {
			pattern.WriteString(regexp.QuoteMeta(path[len(path)-len(rest) : at[0]]))
			pattern.WriteString(`[^/]+`)
			rest = path[at[1]:]
		}
		pattern.WriteString(regexp.QuoteMeta(rest) + "$")
		operations = append(operations, walkOperation{
			route:    route,
			method:   method,
			pattern:  regexp.MustCompile(pattern.String()),
			literals: len(pathParameter.ReplaceAllString(path, "")),
			op:       op,
		})
	}
	// The most literal template first: `/accounts/me` before `/accounts/{id}`.
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].literals != operations[j].literals {
			return operations[i].literals > operations[j].literals
		}
		return operations[i].route < operations[j].route
	})
	return operations, nil
}

var (
	uuidSegment   = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	numberSegment = regexp.MustCompile(`(^|/)\d+(/|:|$)`)
)

// withoutIDs names a path no operation matches without the identifiers in it, which a walk may
// make up anew in every run.
func withoutIDs(path string) string {
	path = uuidSegment.ReplaceAllString(path, "{id}")
	return numberSegment.ReplaceAllString(path, "${1}{n}${2}")
}

// declaredResponse finds what an operation declares for a status: the status itself, its class
// (`4XX`), or `default`.
func (s *specification) declaredResponse(op *operation, status int) (*response, bool) {
	for _, key := range []string{strconv.Itoa(status), fmt.Sprintf("%dXX", status/100), "default"} {
		declared, ok := op.Responses[key]
		if !ok {
			continue
		}
		if name, isRef := strings.CutPrefix(declared.Ref, "#/components/responses/"); isRef {
			declared, ok = s.Components.Responses[name]
		}
		return declared, ok && declared != nil
	}
	return nil, false
}

func isJSON(mediaType string) bool {
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// mediaTypeOf is a content type without its parameters, or "" where the answer names none.
func mediaTypeOf(contentType *string) string {
	if contentType == nil {
		return ""
	}
	return strings.TrimSpace(strings.Split(*contentType, ";")[0])
}

// bodySchema picks the schema an answer's body is held to, and says whether the response allows
// the answer's media type at all. The schema is the one declared for that media type, else the
// declared JSON one - a stub that answers a problem as plain `application/json` is still judged
// as a problem. It is nil for a declared media type that is not JSON: the contract gives the
// lines of `application/x-ndjson` no schema.
func bodySchema(declared *response, mediaType string) (node *schema, allowed bool) {
	if content, ok := declared.Content[mediaType]; ok {
		if isJSON(mediaType) {
			return content.Schema, true
		}
		return nil, true
	}
	if mediaType != "" && !isJSON(mediaType) {
		return nil, false
	}
	mediaTypes := make([]string, 0, len(declared.Content))
	for declaredType := range declared.Content {
		mediaTypes = append(mediaTypes, declaredType)
	}
	sort.Strings(mediaTypes)
	for _, declaredType := range mediaTypes {
		if isJSON(declaredType) {
			return declared.Content[declaredType].Schema, true
		}
	}
	// JSON where the response declares none is outside it; an answer that names no media type
	// has nothing to be held to.
	return nil, mediaType == ""
}

// judgeWalkAnswer answers how one recorded answer leaves the contract, or nil.
func (s *specification) judgeWalkAnswer(operations []walkOperation, answer walkAnswer) *walkViolation {
	var found *walkOperation
	for i := range operations {
		if operations[i].method == answer.Method && operations[i].pattern.MatchString(answer.Path) {
			found = &operations[i]
			break
		}
	}
	if found == nil {
		return &walkViolation{Kind: unknownOperation, Operation: answer.Method + " " + withoutIDs(answer.Path), Status: answer.Status,
			Detail: "the contract declares no such operation"}
	}
	violation := &walkViolation{Operation: found.route, Status: answer.Status}

	declared, ok := s.declaredResponse(found.op, answer.Status)
	if !ok {
		violation.Kind, violation.Detail = statusUndeclared, fmt.Sprintf("%s declares no response %d", found.op.OperationID, answer.Status)
		return violation
	}

	node, allowed := bodySchema(declared, mediaTypeOf(answer.ContentType))
	if !allowed {
		violation.Kind, violation.Detail = contentTypeUndeclared, fmt.Sprintf("%s declares no %s for the response %d", found.op.OperationID, mediaTypeOf(answer.ContentType), answer.Status)
		return violation
	}
	if node == nil || answer.BodyBase64 != "" || answer.BodyFrom != "" {
		return nil
	}
	violation.Kind = bodyMismatch
	if answer.Body == nil || strings.TrimSpace(*answer.Body) == "" {
		violation.Detail = "no body where the contract declares one"
		return violation
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(*answer.Body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		violation.Detail = "the body is not JSON"
		return violation
	}
	problems := s.check("", node, value)
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	if len(problems) > 3 {
		problems = append(problems[:3], fmt.Sprintf("and %d more", len(problems)-3))
	}
	violation.Detail = strings.Join(problems, "; ")
	return violation
}

// readWalkRecords reads every suite's record in a directory: the suite's name to its answers. A
// suite that answered nothing is there with none - it ran.
func readWalkRecords(directory string) (map[string][]walkAnswer, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	records := map[string][]walkAnswer{}
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // G304: a record the walks wrote, named by the caller
		if err != nil {
			return nil, err
		}
		suite := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		records[suite] = []walkAnswer{}
		for n, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var answer walkAnswer
			if err := json.Unmarshal([]byte(line), &answer); err != nil {
				return nil, fmt.Errorf("%s line %d: %w", file, n+1, err)
			}
			records[suite] = append(records[suite], answer)
		}
	}
	return records, nil
}

// judgeWalkRecords answers every violation of the records, one per key, with the suites it was
// met in and the first detail.
func (s *specification) judgeWalkRecords(records map[string][]walkAnswer) (map[string]walkViolation, error) {
	operations, err := s.walkOperations()
	if err != nil {
		return nil, err
	}
	suites := make([]string, 0, len(records))
	for suite := range records {
		suites = append(suites, suite)
	}
	sort.Strings(suites)
	violations := map[string]walkViolation{}
	for _, suite := range suites {
		for _, answer := range records[suite] {
			if answer.Guess {
				continue
			}
			violation := s.judgeWalkAnswer(operations, answer)
			if violation == nil {
				continue
			}
			known, seen := violations[violation.key()]
			if !seen {
				known = *violation
			}
			if !slices.Contains(known.Suites, suite) {
				known.Suites = append(known.Suites, suite)
			}
			violations[violation.key()] = known
		}
	}
	return violations, nil
}

// readWalkBaseline reads the known violations: one key a line, `#` for a comment.
func readWalkBaseline(path string) ([]string, error) {
	file, err := os.Open(path) //nolint:gosec // G304: the committed baseline
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var keys []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			keys = append(keys, line)
		}
	}
	return keys, scanner.Err()
}

// againstBaseline sorts the violations found into those the baseline does not know, and the
// baseline's entries that were not met. An entry that was not met is gone only after a run of
// every suite: one suite or one of CI's shards meets a part of them.
func againstBaseline(found map[string]walkViolation, baseline []string) (unknown []walkViolation, notMet []string) {
	for key, violation := range found {
		if !slices.Contains(baseline, key) {
			unknown = append(unknown, violation)
		}
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i].key() < unknown[j].key() })
	for _, key := range baseline {
		if _, still := found[key]; !still {
			notMet = append(notMet, key)
		}
	}
	return unknown, notMet
}

var leftOut = regexp.MustCompile(`e2e/!\(([^)]+)\)\.test\.mjs`)

// leftOutSuites reads the suites a `node --test` script excludes: `e2e/!(a|b).test.mjs`.
func leftOutSuites(script string) []string {
	match := leftOut.FindStringSubmatch(script)
	if match == nil {
		return nil
	}
	return strings.Split(match[1], "|")
}

// walkedSuites lists the suites a run of every suite walks (`make e2e`): each `*.test.mjs` of the
// web app's e2e directory but those its `test:engines` script leaves out.
func walkedSuites(webapp string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(webapp, "package.json")) //nolint:gosec // G304: the web app's manifest
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%s/package.json: %w", webapp, err)
	}
	script, ok := manifest.Scripts["test:engines"]
	if !ok {
		return nil, fmt.Errorf("%s/package.json has no test:engines script", webapp)
	}
	files, err := filepath.Glob(filepath.Join(webapp, "e2e", "*.test.mjs"))
	if err != nil {
		return nil, err
	}
	excluded := leftOutSuites(script)
	var suites []string
	for _, file := range files {
		if suite := strings.TrimSuffix(filepath.Base(file), ".test.mjs"); !slices.Contains(excluded, suite) {
			suites = append(suites, suite)
		}
	}
	sort.Strings(suites)
	return suites, nil
}

// withoutRecord names the suites that left no record.
func withoutRecord(suites []string, records map[string][]walkAnswer) []string {
	var missing []string
	for _, suite := range suites {
		if _, ok := records[suite]; !ok {
			missing = append(missing, suite)
		}
	}
	return missing
}
