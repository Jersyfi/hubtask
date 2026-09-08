// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command restore-drill is RT-9: the proof that the installation's point-in-time recovery works,
// run where the data is (backup-restore.md §8.5, ADR-0046 amended 2026-09-07).
//
// It writes two marker rows with a recorded moment between them, bootstraps a temporary
// CloudNativePG cluster from the object store to exactly that moment, expects the first marker
// and not the second, runs T-20's consistency and isolation checks against the restored instance,
// measures RPO (how far the archive lags a write) and RTO (how long the restore took to become
// ready), and tears the temporary cluster down whatever happened. It runs after every release as
// a hook and between releases as a CronJob (k8s/templates/job-restore-drill.yaml).
//
// Three things it deliberately is not:
//
//   - Not a Kubernetes client library. It speaks to the API server with net/http and the token
//     the pod was given; creating a Cluster is one POST. A client library would be a supply chain
//     decision for a tool that makes four kinds of request (ADR-0015).
//   - Not a source of numbers. The measured RPO and RTO go into the evidence, and the evidence
//     goes where the operator says (an object store, or the record ConfigMap when none is named).
//     What it prints is a pass/fail line with a run identifier - decision 7 of the 0.6.0 backlog
//     applies to a log line in Loki as much as to a document in this repository.
//   - Not a test of the application. The checks are about the database that came back: the
//     recovery target, the schema version, the row level security every tenant table has to
//     carry, the application role's bounds. Whether the application works on top of it is what
//     every other gate proves.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "hubtask"),
		slog.String("role", "restore-drill"),
	)

	cfg, err := loadConfig(os.Getenv, os.ReadFile)
	if err != nil {
		logger.Error("the drill is not configured", slog.String("error", err.Error()))
		os.Exit(2)
	}

	kube, err := newKubeClient(cfg.Kube, cfg.Namespace)
	if err != nil {
		logger.Error("the Kubernetes API is not reachable from this pod", slog.String("error", err.Error()))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeouts.Total)
	defer cancel()

	if err := newDrill(cfg, kube, logger).run(ctx); err != nil {
		// The drill ran and found something. Whether that ends the release is a decision, not a
		// property of the failure - see config.FailRelease. The finding is already in the record,
		// in the alerts that read it, and in the error line the run wrote.
		if cfg.FailRelease {
			os.Exit(1)
		}
		logger.Warn("the drill failed and this job reports success by configuration",
			slog.String("error_code", "restore_drill.failed"),
			slog.String("remedy", "set restoreDrill.failRelease to stop a release on this"))
	}
}
