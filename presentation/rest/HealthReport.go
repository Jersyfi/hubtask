// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The wire format of GET /meta/health, schema HealthReport in api/openapi.yaml. Hand-written for
// now; from A-06 onwards these types are generated from the specification and this file goes
// (ADR-0004). The contract test compares the response against the schema either way, so the two
// cannot drift silently.
//
// Everything optional is a pointer: the schema distinguishes "no latency measured" from "zero
// milliseconds", and a status page reading 0 ms for a dependency that was never reached would
// draw the wrong conclusion.
type healthReport struct {
	Status           string             `json:"status"`
	Version          string             `json:"version"`
	Role             []string           `json:"role,omitempty"`
	Migration        *migrationState    `json:"migration,omitempty"`
	Dependencies     []dependencyHealth `json:"dependencies,omitempty"`
	DegradedFeatures []degradedFeature  `json:"degraded_features,omitempty"`
	Backlogs         *backlogs          `json:"backlogs,omitempty"`
	Warnings         []healthWarning    `json:"warnings,omitempty"`
}

type migrationState struct {
	Applied  int    `json:"applied"`
	Expected int    `json:"expected"`
	Status   string `json:"status"`
}

type dependencyHealth struct {
	Name          string   `json:"name"`
	Required      bool     `json:"required"`
	Status        string   `json:"status"`
	LatencyMS     *float64 `json:"latency_ms,omitempty"`
	Since         *string  `json:"since,omitempty"`
	LastErrorCode *string  `json:"last_error_code,omitempty"`
	CircuitState  *string  `json:"circuit_state,omitempty"`
	Impact        []string `json:"impact,omitempty"`
}

type degradedFeature struct {
	Feature    string `json:"feature"`
	ReasonCode string `json:"reason_code"`
	Since      string `json:"since"`
}

type backlogs struct {
	OutboxPending       int64   `json:"outbox_pending"`
	OutboxLagSeconds    float64 `json:"outbox_lag_seconds"`
	JobQueueDepth       int64   `json:"job_queue_depth"`
	DeadLetterTotal     int64   `json:"dead_letter_total"`
	WebhookRetryBacklog int64   `json:"webhook_retry_backlog"`
}

type healthWarning struct {
	Code     string            `json:"code"`
	Severity string            `json:"severity"`
	Params   map[string]string `json:"params,omitempty"`
}

func healthReportJSON(report health.Report) healthReport {
	out := healthReport{
		Status:       string(report.Status),
		Version:      report.Version,
		Role:         report.Roles,
		Dependencies: make([]dependencyHealth, 0, len(report.Dependencies)),
	}

	if report.Migration != (health.MigrationState{}) {
		out.Migration = &migrationState{
			Applied:  report.Migration.Applied,
			Expected: report.Migration.Expected,
			Status:   report.Migration.Status,
		}
	}
	if report.Backlogs != (health.Backlogs{}) {
		out.Backlogs = &backlogs{
			OutboxPending:       report.Backlogs.OutboxPending,
			OutboxLagSeconds:    report.Backlogs.OutboxLagSeconds,
			JobQueueDepth:       report.Backlogs.JobQueueDepth,
			DeadLetterTotal:     report.Backlogs.DeadLetterTotal,
			WebhookRetryBacklog: report.Backlogs.WebhookRetryBacklog,
		}
	}

	for _, d := range report.Dependencies {
		entry := dependencyHealth{
			Name:     d.Name,
			Required: d.Required,
			Status:   string(d.Status),
			Impact:   d.Impact,
		}
		if d.Latency > 0 {
			ms := float64(d.Latency.Nanoseconds()) / float64(time.Millisecond)
			entry.LatencyMS = &ms
		}
		if !d.Since.IsZero() {
			since := d.Since.UTC().Format(time.RFC3339)
			entry.Since = &since
		}
		if d.ErrorCode != "" {
			// A code, never a driver message: this endpoint is read by a status page
			// (security.md §9).
			code := d.ErrorCode
			entry.LastErrorCode = &code
		}
		if d.CircuitState != "" {
			state := d.CircuitState
			entry.CircuitState = &state
		}
		out.Dependencies = append(out.Dependencies, entry)
	}

	for _, f := range report.DegradedFeatures {
		out.DegradedFeatures = append(out.DegradedFeatures, degradedFeature{
			Feature:    f.Feature,
			ReasonCode: f.ReasonCode,
			Since:      f.Since.UTC().Format(time.RFC3339),
		})
	}

	for _, w := range report.Warnings {
		out.Warnings = append(out.Warnings, healthWarning{
			Code:     w.Code,
			Severity: w.Severity,
			Params:   w.Params,
		})
	}

	return out
}

// HealthReportReader is the slice of the use case this controller needs (K-06).
type HealthReportReader interface {
	Execute(context.Context, appshared.ActorContext) (health.Report, error)
}

// GetHealthReport answers GET /api/v1/meta/health, the door the contract has declared since A-06
// and `Pending.go` answered `404` to until K-06 (#507).
//
// It answers `200` even when the status is `down`, which the internal listener deliberately does
// not: the contract says in its own description that "the HTTP status describes whether the
// endpoint is reachable, the `status` field describes the system", and a client that receives a
// `503` here cannot tell a report that says "down" from a gateway that never reached this process.
// The operations listener keeps its `503` because a status page reads a status code and holds no
// token. The two differ on purpose.
//
// Who sees how much is not decided here. The use case answers the whole report or the reduced one,
// because that is an authorisation decision and those live in the application layer (rule 2).
func (c *RestController) GetHealthReport(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.HealthReport == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	report, err := c.HealthReport.Execute(r.Context(), actorOf(r))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	writeJSON(w, r, http.StatusOK, healthReportJSON(report))
}
