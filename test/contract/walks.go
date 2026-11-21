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
}

// The three ways an answer leaves the contract.
const (
	unknownOperation = "unknown-operation"
	statusUndeclared = "status-undeclared"
	bodyMismatch     = "body-mismatch"
)

// walkViolation is one answer the contract does not allow. Its key names the suite, the kind, the
// operation and the status - never the body, so that it reads the same in every run.
type walkViolation struct {
	Suite     string
	Kind      string
	Operation string
	Status    int
	Detail    string
}

func (v walkViolation) key() string {
	return fmt.Sprintf("%s %s %s %d", v.Suite, v.Kind, v.Operation, v.Status)
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

// bodySchema picks the schema an answer's body is held to: the one declared for its own media
// type, else the declared JSON one - a stub that answers a problem as plain `application/json` is
// still judged as a problem. Nil where the contract declares no JSON body for the status.
func bodySchema(declared *response, contentType *string) *schema {
	if contentType != nil {
		mediaType := strings.TrimSpace(strings.Split(*contentType, ";")[0])
		if content, ok := declared.Content[mediaType]; ok {
			if isJSON(mediaType) {
				return content.Schema
			}
			return nil
		}
		if !isJSON(mediaType) && mediaType != "" {
			return nil
		}
	}
	mediaTypes := make([]string, 0, len(declared.Content))
	for mediaType := range declared.Content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	for _, mediaType := range mediaTypes {
		if isJSON(mediaType) {
			return declared.Content[mediaType].Schema
		}
	}
	return nil
}

// judgeWalkAnswer answers how one recorded answer leaves the contract, or nil.
func (s *specification) judgeWalkAnswer(operations []walkOperation, suite string, answer walkAnswer) *walkViolation {
	var found *walkOperation
	for i := range operations {
		if operations[i].method == answer.Method && operations[i].pattern.MatchString(answer.Path) {
			found = &operations[i]
			break
		}
	}
	if found == nil {
		return &walkViolation{Suite: suite, Kind: unknownOperation, Operation: answer.Method + " " + withoutIDs(answer.Path), Status: answer.Status,
			Detail: "the contract declares no such operation"}
	}
	violation := &walkViolation{Suite: suite, Operation: found.route, Status: answer.Status}

	declared, ok := s.declaredResponse(found.op, answer.Status)
	if !ok {
		violation.Kind, violation.Detail = statusUndeclared, fmt.Sprintf("%s declares no response %d", found.op.OperationID, answer.Status)
		return violation
	}

	node := bodySchema(declared, answer.ContentType)
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

// judgeWalkRecords answers every violation of the records, one per key, with the first detail met.
func (s *specification) judgeWalkRecords(records map[string][]walkAnswer) (map[string]walkViolation, error) {
	operations, err := s.walkOperations()
	if err != nil {
		return nil, err
	}
	violations := map[string]walkViolation{}
	for suite, answers := range records {
		for _, answer := range answers {
			violation := s.judgeWalkAnswer(operations, suite, answer)
			if violation == nil {
				continue
			}
			if _, seen := violations[violation.key()]; !seen {
				violations[violation.key()] = *violation
			}
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
// baseline's entries that no longer occur. Only a suite with a record can be said to have lost a
// violation: the walks run in shards, and a shard knows nothing about the other suites.
func againstBaseline(found map[string]walkViolation, baseline []string, recorded map[string][]walkAnswer) (unknown []walkViolation, gone []string) {
	for key, violation := range found {
		if !slices.Contains(baseline, key) {
			unknown = append(unknown, violation)
		}
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i].key() < unknown[j].key() })
	for _, key := range baseline {
		suite, _, _ := strings.Cut(key, " ")
		if _, ran := recorded[suite]; !ran {
			continue
		}
		if _, still := found[key]; !still {
			gone = append(gone, key)
		}
	}
	return unknown, gone
}
