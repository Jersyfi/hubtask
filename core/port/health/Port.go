// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package health carries the health model from ADR-0016.
//
// Four levels, deliberately kept separate:
//
//	/healthz   - the process is alive. NEVER checks dependencies, otherwise a database
//	             outage takes down every pod at once.
//	/startupz  - initialisation is complete.
//	/readyz    - the process can serve traffic.
//	/api/v1/meta/health - deep self-diagnosis including configuration warnings.
package health

import (
	"context"
	"time"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
	StatusDisabled Status = "disabled"
)

// Probe describes a monitored dependency.
type Probe interface {
	// Name is stable and appears in metrics - so keep it short and without spaces.
	Name() string
	// Required separates mandatory (PostgreSQL) from optional (S3, SMTP, AI).
	// The failure of an optional dependency must never block the write path.
	Required() bool
	Check(ctx context.Context) Result
}

type Result struct {
	Status       Status
	Latency      time.Duration
	ErrorCode    string
	Since        time.Time
	CircuitState string   // closed | half_open | open
	Impact       []string // features affected during a disruption
}

// Report is the response of /api/v1/meta/health.
type Report struct {
	Status           Status
	Version          string
	Roles            []string
	Migration        MigrationState
	Dependencies     []DependencyReport
	DegradedFeatures []DegradedFeature
	Backlogs         Backlogs
	Warnings         []Warning
}

type MigrationState struct {
	Applied  int
	Expected int
	Status   string // ok | behind | ahead
}

type DependencyReport struct {
	Name     string
	Required bool
	Result
}

type DegradedFeature struct {
	Feature    string
	ReasonCode string
	Since      time.Time
}

type Backlogs struct {
	OutboxPending       int64
	OutboxLagSeconds    float64
	JobQueueDepth       int64
	DeadLetterTotal     int64
	WebhookRetryBacklog int64
}

type Warning struct {
	Code     string
	Severity string
	Params   map[string]string
}

// Registry collects probes and produces the report.
type Registry interface {
	Register(p Probe)
	Live() bool
	Ready(ctx context.Context) (bool, string)
	Report(ctx context.Context) Report
}
