// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"strings"
	"testing"
)

// A contract test that cannot fail proves nothing, so the validator is checked against
// deliberately wrong responses first - the same idea as make gate-selftest.
func TestTheValidatorCatchesABrokenResponse(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("the specification could not be read: %v", err)
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "a required field is missing",
			body: `{"version":"0.1.0","dependencies":[]}`,
			want: `required field "status"`,
		},
		{
			name: "a value outside the enum",
			body: `{"status":"probably fine","version":"0.1.0","dependencies":[]}`,
			want: "outside the enum",
		},
		{
			name: "the wrong type",
			body: `{"status":"ok","version":7,"dependencies":[]}`,
			want: "is not string",
		},
		{
			name: "a field nobody declared",
			body: `{"status":"ok","version":"0.1.0","dependencies":[],"db_password":"hunter2"}`,
			want: `"db_password" is not in the specification`,
		},
		{
			name: "a broken dependency entry",
			body: `{"status":"ok","version":"0.1.0","dependencies":[{"name":"postgres"}]}`,
			want: `required field "required"`,
		},
		{
			name: "a circuit state that does not exist",
			body: `{"status":"ok","version":"0.1.0","dependencies":[` +
				`{"name":"s3","required":false,"status":"down","circuit_state":"melted"}]}`,
			want: "outside the enum",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems, err := spec.validateAgainst("HealthReport", []byte(tc.body))
			if err != nil {
				t.Fatalf("validating: %v", err)
			}
			if len(problems) == 0 {
				t.Fatalf("the validator accepted %s", tc.body)
			}
			if !strings.Contains(strings.Join(problems, "; "), tc.want) {
				t.Errorf("the findings %v do not mention %q", problems, tc.want)
			}
		})
	}
}

// And the reverse: a report exercising every optional field has to pass, or the validator is
// merely strict rather than correct.
func TestTheValidatorAcceptsAFullReport(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatalf("the specification could not be read: %v", err)
	}

	body := `{
	  "status": "degraded",
	  "version": "0.1.0",
	  "role": ["api", "worker"],
	  "migration": {"applied": 47, "expected": 47, "status": "ok"},
	  "dependencies": [
	    {"name": "postgres", "required": true, "status": "ok", "latency_ms": 3},
	    {"name": "object_storage", "required": false, "status": "down",
	     "since": "2026-08-14T09:12:04Z", "last_error_code": "storage.unreachable",
	     "circuit_state": "open", "impact": ["media.upload"]},
	    {"name": "ai_provider", "required": false, "status": "disabled",
	     "latency_ms": null, "since": null, "last_error_code": null, "circuit_state": null}
	  ],
	  "degraded_features": [
	    {"feature": "media", "reason_code": "dependency.unavailable", "since": "2026-08-14T09:12:04Z"}
	  ],
	  "backlogs": {"outbox_pending": 12, "outbox_lag_seconds": 1.5, "job_queue_depth": 3,
	               "dead_letter_total": 0, "webhook_retry_backlog": 0},
	  "warnings": [
	    {"code": "config.backup_not_configured", "severity": "warn"},
	    {"code": "config.smtp_without_tls", "severity": "warn", "params": {"host": "mail.example.org"}}
	  ]
	}`

	problems, err := spec.validateAgainst("HealthReport", []byte(body))
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	for _, p := range problems {
		t.Errorf("a valid report was rejected: %s", p)
	}
}
