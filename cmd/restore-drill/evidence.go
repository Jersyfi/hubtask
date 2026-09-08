// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/storage"
)

const (
	resultPass = "pass"
	resultFail = "fail"
)

// evidence is what one run measured and found. It is written whole to the operator's location
// and, without one, into the record ConfigMap; the numbers in it never reach a log line.
type evidence struct {
	RunID            string    `json:"run_id"`
	Release          string    `json:"release,omitempty"`
	SourceCluster    string    `json:"source_cluster"`
	TemporaryCluster string    `json:"temporary_cluster"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	Result           string    `json:"result"`
	Error            string    `json:"error,omitempty"`
	TargetTime       time.Time `json:"target_time,omitempty"`
	SchemaVersion    int64     `json:"schema_version,omitempty"`
	RPO              rpoSample `json:"rpo"`
	RTO              rtoSample `json:"rto"`
	Checks           []check   `json:"checks,omitempty"`
}

// rpoSample is how far the archive was from the live database when the drill looked.
type rpoSample struct {
	// FirstRecoverabilityPoint is the earliest moment the archive can restore to.
	FirstRecoverabilityPoint time.Time `json:"first_recoverability_point,omitempty"`
	// ArchiveLagAtDrillSeconds is the steady state: how old the last archived segment was
	// before the drill touched anything. Absent when nothing was ever archived.
	ArchiveLagAtDrillSeconds *float64 `json:"archive_lag_at_drill_seconds,omitempty"`
	// WALArchiveWaitSeconds is how long the second marker's segment took to reach the archive
	// once it was closed: the distance between a write and its being recoverable.
	WALArchiveWaitSeconds *float64 `json:"wal_archive_wait_seconds,omitempty"`
	// ArchiveFailuresDuringDrill counts archiver failures while the drill waited.
	ArchiveFailuresDuringDrill int64 `json:"archive_failures_during_drill"`
}

// rtoSample is how long the recovery took.
type rtoSample struct {
	// RestoreSeconds runs from asking for the temporary cluster to the operator calling it ready.
	RestoreSeconds float64 `json:"restore_seconds,omitempty"`
	// VerificationSeconds is the checks on top; an operator's recovery would spend it too.
	VerificationSeconds float64 `json:"verification_seconds,omitempty"`
}

func (e evidence) passed() bool {
	if len(e.Checks) == 0 {
		return false
	}
	for _, c := range e.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func (e evidence) document() ([]byte, error) {
	body, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding the evidence: %w", err)
	}
	return body, nil
}

// summary is the record's line: the run, when, and whether. Nothing measured.
func (e evidence) summary() string {
	body, _ := json.Marshal(map[string]string{
		"run_id":      e.RunID,
		"finished_at": e.FinishedAt.UTC().Format(time.RFC3339),
		"result":      e.Result,
		"release":     e.Release,
	})
	return string(body)
}

// redacted is the one line the log gets: pass or fail, which check, and where the evidence went.
// No duration appears here on purpose - a log is read by more people than the evidence is.
func (e evidence) redacted(where string) []slog.Attr {
	passed, total := 0, len(e.Checks)
	failedCheck := ""
	for _, c := range e.Checks {
		if c.OK {
			passed++
		} else if failedCheck == "" {
			failedCheck = c.Name
		}
	}
	attrs := []slog.Attr{
		slog.String("result", e.Result),
		slog.String("checks", strconv.Itoa(passed)+"/"+strconv.Itoa(total)),
		slog.String("evidence", where),
	}
	if failedCheck != "" {
		attrs = append(attrs, slog.String("failed_check", failedCheck))
	}
	if e.Error != "" {
		attrs = append(attrs, slog.String("error_code", "restore_drill.failed"), slog.String("error", e.Error))
	}
	return attrs
}

// publish writes the evidence to the operator's object store when one is named; otherwise the
// record carries it and this says so.
func (d *drill) publish(ctx context.Context, ev evidence) (string, error) {
	if !d.cfg.Evidence.configured() {
		return "configmap:" + d.cfg.RecordConfigMap, nil
	}
	body, err := ev.document()
	if err != nil {
		return "", err
	}
	store, err := storage.NewS3Storage(env.StorageConfig{
		Kind:         env.StorageS3,
		Endpoint:     d.cfg.Evidence.Endpoint,
		Region:       d.cfg.Evidence.Region,
		Bucket:       d.cfg.Evidence.Bucket,
		AccessKey:    d.cfg.Evidence.AccessKey,
		SecretKey:    d.cfg.Evidence.SecretKey,
		UsePathStyle: d.cfg.Evidence.UsePathStyle,
	}, 30*time.Second)
	if err != nil {
		return "", err
	}
	key := d.cfg.Evidence.evidenceKey(ev.RunID)
	writing, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := store.Put(writing, port.Upload{
		Key:         key,
		Content:     bytes.NewReader(body),
		Size:        int64(len(body)),
		ContentType: "application/json",
	}); err != nil {
		return "", fmt.Errorf("writing the evidence to the object store: %w", err)
	}
	return "s3:" + key, nil
}
