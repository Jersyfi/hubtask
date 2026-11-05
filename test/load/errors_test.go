// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

package load

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/test/load/harness"
)

// The run's scenarios and the browser's rows, kept for the one collection TestMain makes after the
// last scenario (test/load/PLAN.md, "How the errors are collected").
var (
	runMu        sync.Mutex
	runScenarios []harness.Scenario
	browserRows  []harness.LogError
)

// scenario records one window of a scenario; the answered function closes it.
func scenario(name string) (end func()) {
	start := time.Now().UTC()
	return func() {
		runMu.Lock()
		defer runMu.Unlock()
		runScenarios = append(runScenarios, harness.Scenario{Name: name, Start: start, End: time.Now().UTC()})
	}
}

// keepBrowserRows adds a walk's rows, which carry no time of their own, to the scenario that
// walked.
func keepBrowserRows(name string, rows []harness.LogError) {
	runMu.Lock()
	defer runMu.Unlock()
	for _, row := range rows {
		row.Scenarios = []string{name}
		browserRows = append(browserRows, row)
	}
}

// collectRunErrors reads the whole server log and PostgreSQL's log once and writes
// errors-<items>.json: each row with its first and last time and the scenarios it fell in, the
// expected rows apart from the findings.
func collectRunErrors(stack *stack) error {
	runMu.Lock()
	scenarios := append([]harness.Scenario(nil), runScenarios...)
	rows := append([]harness.LogError(nil), browserRows...)
	runMu.Unlock()

	file, err := os.Open(stack.serverLog)
	if err != nil {
		return fmt.Errorf("opening the server's log: %w", err)
	}
	server, err := harness.ServerErrors(file, scenarios)
	_ = file.Close()
	if err != nil {
		return fmt.Errorf("reading the server's log: %w", err)
	}
	rows = append(rows, server...)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	logs, err := stack.database.Logs(ctx)
	if err != nil {
		return fmt.Errorf("reading PostgreSQL's log: %w", err)
	}
	postgres, err := harness.PostgresErrors(logs, scenarios)
	_ = logs.Close()
	if err != nil {
		return fmt.Errorf("reading PostgreSQL's log: %w", err)
	}
	rows = append(rows, postgres...)

	report := harness.Report(scenarios, rows)
	directory := os.Getenv(envEvidence)
	if directory == "" {
		directory = os.TempDir()
	}
	path, err := writeEvidenceFile(directory, fmt.Sprintf("errors-%d.json", stack.items), report)
	if err != nil {
		return err
	}
	for _, row := range report.Findings {
		fmt.Fprintf(os.Stderr, "finding: %s %s ×%d %v: %s\n", row.Source, row.Level, row.Count, row.Scenarios, row.Message)
	}
	for _, row := range report.Expected {
		fmt.Fprintf(os.Stderr, "expected: %s %s ×%d %v: %s\n", row.Source, row.Level, row.Count, row.Scenarios, row.Message)
	}
	fmt.Fprintf(os.Stderr, "the run's errors are in %s: %d findings, %d expected rows over %d scenario windows\n",
		path, len(report.Findings), len(report.Expected), len(scenarios))
	return nil
}
