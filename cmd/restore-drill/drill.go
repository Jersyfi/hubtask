// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// drill is one run. Its clock and its sleep are fields so that a test can drive the pure parts;
// everything that touches a database or the API server is a method on it.
type drill struct {
	cfg  config
	kube *kubeClient
	log  *slog.Logger
	now  func() time.Time
}

func newDrill(cfg config, kube *kubeClient, log *slog.Logger) *drill {
	return &drill{cfg: cfg, kube: kube, log: log, now: time.Now}
}

// targetTimeLayout is how CloudNativePG wants recoveryTarget.targetTime: PostgreSQL's own
// timestamp shape with microseconds and a numeric zone, in UTC because the markers are.
const targetTimeLayout = "2006-01-02 15:04:05.000000-07"

// markerGap is the room left on each side of the recovery target. Commit timestamps and the
// server's clock read in between are the same clock, so anything above its resolution would do;
// a second on each side makes the ordering unarguable in a log somebody reads later.
const markerGap = 1500 * time.Millisecond

// walSegmentName is what pg_walfile_name returns: timeline, log and segment, eight hex digits
// each. Names on one timeline sort lexically in the order they were written.
var walSegmentName = regexp.MustCompile(`^[0-9A-F]{24}$`)

// run is the whole drill. Whatever happens after the temporary cluster is asked for, the teardown
// runs; whatever happens at all, the record and the evidence are written - a drill that failed to
// say it failed would leave A-20 reading the previous success.
func (d *drill) run(ctx context.Context) (err error) {
	ev := evidence{
		RunID:            d.now().UTC().Format("20060102-150405"),
		Release:          d.cfg.Release,
		SourceCluster:    d.cfg.SourceCluster,
		TemporaryCluster: d.cfg.Temporary.Name,
		StartedAt:        d.now().UTC(),
	}
	log := d.log.With(slog.String("run_id", ev.RunID))

	defer func() {
		ev.FinishedAt = d.now().UTC()
		if err != nil {
			ev.Result = resultFail
			ev.Error = err.Error()
		} else if !ev.passed() {
			ev.Result = resultFail
			err = errors.New("a check failed")
			ev.Error = err.Error()
		} else {
			ev.Result = resultPass
		}
		// The record and the evidence get a context of their own: the run's may be the reason
		// this is a failure, and a record not written is a gauge that lies.
		recording, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		where, publishErr := d.publish(recording, ev)
		if publishErr != nil {
			log.Error("the evidence could not be written", slog.String("error", publishErr.Error()))
			if err == nil {
				err = publishErr
			}
		}
		if recordErr := d.record(recording, ev); recordErr != nil {
			log.Error("the record could not be written", slog.String("error", recordErr.Error()))
			if err == nil {
				err = recordErr
			}
		}
		log.LogAttrs(ctx, resultLevel(err), "restore drill finished", ev.redacted(where)...)
	}()

	live, err := d.connect(ctx, d.cfg.OwnerDSN)
	if err != nil {
		return fmt.Errorf("the live database is unreachable as the owner: %w", err)
	}
	defer func() { _ = live.Close(context.WithoutCancel(ctx)) }()

	if err := d.pruneMarkers(ctx, live); err != nil {
		return err
	}

	// A leftover from a run that died before its teardown. Removed first, so that the quota is
	// free and the name is ours.
	if err := d.teardown(ctx, log, true); err != nil {
		return fmt.Errorf("a previous temporary cluster could not be removed: %w", err)
	}

	firstRecoverable, err := d.waitForRecoverabilityPoint(ctx, log)
	if err != nil {
		return err
	}
	ev.RPO.FirstRecoverabilityPoint = firstRecoverable

	target, err := d.writeMarkers(ctx, live, ev.RunID)
	if err != nil {
		return err
	}
	ev.TargetTime = target
	log.Info("markers written", slog.String("target_time", target.Format(time.RFC3339Nano)))

	if err := d.waitForArchive(ctx, log, live, &ev.RPO); err != nil {
		return err
	}

	if ev.SchemaVersion, err = schemaVersion(ctx, live); err != nil {
		return fmt.Errorf("reading the live schema version: %w", err)
	}

	restoreStarted := d.now()
	if err := d.kube.createCluster(ctx, temporaryClusterManifest(d.cfg, target)); err != nil {
		return err
	}
	defer func() {
		if err != nil || !ev.passed() {
			if d.cfg.KeepOnFailure {
				log.Warn("the temporary cluster is kept for inspection", slog.String("cluster", d.cfg.Temporary.Name))
				return
			}
		}
		if teardownErr := d.teardown(context.WithoutCancel(ctx), log, false); teardownErr != nil {
			log.Error("the temporary cluster could not be torn down", slog.String("error", teardownErr.Error()))
			if err == nil {
				err = teardownErr
			}
		}
	}()

	if err := d.waitForTemporary(ctx, log); err != nil {
		return err
	}
	ev.RTO.RestoreSeconds = d.now().Sub(restoreStarted).Seconds()
	log.Info("the temporary cluster is ready")

	verifyStarted := d.now()
	ev.Checks, err = d.verify(ctx, ev.RunID, ev.SchemaVersion)
	if err != nil {
		return err
	}
	ev.RTO.VerificationSeconds = d.now().Sub(verifyStarted).Seconds()
	return nil
}

// connect opens one connection, retrying for the connect timeout: a service that was created a
// moment ago has no endpoints yet, and the first DNS answer for it is a refusal.
func (d *drill) connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	deadline := d.now().Add(d.cfg.Timeouts.Connect)
	for {
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		conn, err := pgx.Connect(attempt, dsn)
		cancel()
		if err == nil {
			return conn, nil
		}
		if d.now().After(deadline) {
			return nil, err
		}
		if err := sleep(ctx, 5*time.Second); err != nil {
			return nil, err
		}
	}
}

func sleep(ctx context.Context, wait time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// pruneMarkers keeps the marker table small: rows older than the retention go, and nothing else
// ever deletes from it.
func (d *drill) pruneMarkers(ctx context.Context, live *pgx.Conn) error {
	_, err := live.Exec(ctx, `DELETE FROM restore_drill_marker WHERE written_at < now() - $1::interval`,
		d.cfg.MarkerRetention.String())
	if err != nil {
		return fmt.Errorf("pruning old markers: %w", err)
	}
	return nil
}

// waitForRecoverabilityPoint waits until the source cluster reports a first base backup. A
// fresh installation's first drill runs minutes after its ScheduledBackup was created, and the
// recovery target has to lie after the backup's end.
func (d *drill) waitForRecoverabilityPoint(ctx context.Context, log *slog.Logger) (time.Time, error) {
	deadline := d.now().Add(d.cfg.Timeouts.BackupWait)
	warned := false
	for {
		found, cluster, err := d.kube.getCluster(ctx, d.cfg.SourceCluster)
		if err != nil {
			return time.Time{}, err
		}
		if !found {
			return time.Time{}, fmt.Errorf("the source cluster %s does not exist", d.cfg.SourceCluster)
		}
		if point, ok := recoverabilityPoint(cluster); ok {
			return point, nil
		}
		if d.now().After(deadline) {
			return time.Time{}, errors.New("the source cluster has no base backup to recover from")
		}
		if !warned {
			log.Info("waiting for the first base backup")
			warned = true
		}
		if err := sleep(ctx, 10*time.Second); err != nil {
			return time.Time{}, err
		}
	}
}

// recoverabilityPoint reads status.firstRecoverabilityPoint, which the operator sets once the
// first base backup has completed.
func recoverabilityPoint(cluster map[string]any) (time.Time, bool) {
	status, _ := cluster["status"].(map[string]any)
	raw, _ := status["firstRecoverabilityPoint"].(string)
	if raw == "" {
		return time.Time{}, false
	}
	point, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return point, true
}

// writeMarkers is the two writes the recovery target has to fall between. The moment is read
// from the server after the first commit and before the second, with the gap on each side, so
// that the recovery target and the commits it is compared against share one clock.
func (d *drill) writeMarkers(ctx context.Context, live *pgx.Conn, runID string) (time.Time, error) {
	if _, err := live.Exec(ctx, `INSERT INTO restore_drill_marker (run_id, seq) VALUES ($1, 1)`, runID); err != nil {
		return time.Time{}, fmt.Errorf("writing the first marker: %w", err)
	}
	if err := sleep(ctx, markerGap); err != nil {
		return time.Time{}, err
	}
	var target time.Time
	if err := live.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&target); err != nil {
		return time.Time{}, fmt.Errorf("reading the server clock: %w", err)
	}
	if err := sleep(ctx, markerGap); err != nil {
		return time.Time{}, err
	}
	if _, err := live.Exec(ctx, `INSERT INTO restore_drill_marker (run_id, seq) VALUES ($1, 2)`, runID); err != nil {
		return time.Time{}, fmt.Errorf("writing the second marker: %w", err)
	}
	return target.UTC(), nil
}

// waitForArchive is the RPO sample. The archive's lag before the drill touched anything is the
// steady state; then the segment holding the second marker is closed and the wait until the
// archiver reports it is how far a write is from being recoverable.
func (d *drill) waitForArchive(ctx context.Context, log *slog.Logger, live *pgx.Conn, rpo *rpoSample) error {
	var lastArchived *time.Time
	if err := live.QueryRow(ctx, `SELECT last_archived_time FROM pg_stat_archiver`).Scan(&lastArchived); err != nil {
		return fmt.Errorf("reading the archiver: %w", err)
	}
	if lastArchived != nil {
		lag := d.now().Sub(*lastArchived).Seconds()
		rpo.ArchiveLagAtDrillSeconds = &lag
	}

	var segment string
	if err := live.QueryRow(ctx, `SELECT pg_walfile_name(pg_current_wal_lsn())`).Scan(&segment); err != nil {
		return fmt.Errorf("reading the current WAL segment: %w", err)
	}
	if !walSegmentName.MatchString(segment) {
		return fmt.Errorf("pg_walfile_name answered %q, which is not a segment name", segment)
	}
	switched := d.now()
	if _, err := live.Exec(ctx, `SELECT pg_switch_wal()`); err != nil {
		return fmt.Errorf("switching the WAL segment: %w", err)
	}

	deadline := switched.Add(d.cfg.Timeouts.ArchiveWait)
	var failuresBefore int64
	if err := live.QueryRow(ctx, `SELECT failed_count FROM pg_stat_archiver`).Scan(&failuresBefore); err != nil {
		return fmt.Errorf("reading the archiver: %w", err)
	}
	for {
		var archived *string
		var failures int64
		if err := live.QueryRow(ctx, `SELECT last_archived_wal, failed_count FROM pg_stat_archiver`).Scan(&archived, &failures); err != nil {
			return fmt.Errorf("reading the archiver: %w", err)
		}
		if archived != nil && segmentArchived(*archived, segment) {
			wait := d.now().Sub(switched).Seconds()
			rpo.WALArchiveWaitSeconds = &wait
			rpo.ArchiveFailuresDuringDrill = failures - failuresBefore
			return nil
		}
		if d.now().After(deadline) {
			rpo.ArchiveFailuresDuringDrill = failures - failuresBefore
			return fmt.Errorf("the WAL segment %s was not archived within %s", segment, d.cfg.Timeouts.ArchiveWait)
		}
		if failures > failuresBefore {
			log.Warn("the archiver reported a failure while the drill waited")
			failuresBefore = failures
		}
		if err := sleep(ctx, 5*time.Second); err != nil {
			return err
		}
	}
}

// segmentArchived says whether the archiver's last segment is at or past the one asked about.
// Names on one timeline sort lexically; a later timeline sorts after every earlier one.
func segmentArchived(last, wanted string) bool {
	return walSegmentName.MatchString(last) && last >= wanted
}

// temporaryClusterManifest is the Cluster the drill bootstraps: a recovery from the live
// cluster's archive to the target, with no backup stanza of its own - a restored cluster that
// archived under the live cluster's name would overwrite the archive it was restored from.
func temporaryClusterManifest(cfg config, target time.Time) map[string]any {
	external := map[string]any{
		"destinationPath": cfg.Archive.DestinationPath,
		"serverName":      cfg.Archive.ServerName,
		"s3Credentials": map[string]any{
			"accessKeyId":     map[string]any{"name": cfg.Archive.Secret, "key": "access-key"},
			"secretAccessKey": map[string]any{"name": cfg.Archive.Secret, "key": "secret-key"},
		},
		"wal": map[string]any{"compression": cfg.Archive.Compression},
	}
	if cfg.Archive.EndpointURL != "" {
		external["endpointURL"] = cfg.Archive.EndpointURL
	}

	storage := map[string]any{"size": cfg.Temporary.StorageSize}
	if cfg.Temporary.StorageClass != "" {
		storage["storageClass"] = cfg.Temporary.StorageClass
	}

	spec := map[string]any{
		"instances": 1,
		"imageName": cfg.Temporary.ImageName,
		"storage":   storage,
		"bootstrap": map[string]any{
			"recovery": map[string]any{
				"source":   "live",
				"database": cfg.Temporary.DatabaseName,
				"owner":    cfg.Temporary.Owner,
				// The recovered owner gets the live cluster's password, so the live DSN works
				// against the temporary cluster with only its host swapped.
				"secret": map[string]any{"name": cfg.Temporary.OwnerSecret},
				"recoveryTarget": map[string]any{
					"targetTime": target.UTC().Format(targetTimeLayout),
				},
			},
		},
		"externalClusters": []any{
			map[string]any{"name": "live", "barmanObjectStore": external},
		},
	}
	if cfg.Temporary.ResourcesJSON != "" {
		var resources map[string]any
		if err := json.Unmarshal([]byte(cfg.Temporary.ResourcesJSON), &resources); err == nil {
			spec["resources"] = resources
		}
	}

	return map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Cluster",
		"metadata": map[string]any{
			"name":      cfg.Temporary.Name,
			"namespace": cfg.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/part-of":   "hubtask",
				"app.kubernetes.io/component": "restore-drill",
			},
		},
		"spec": spec,
	}
}

// waitForTemporary polls the temporary cluster until the operator calls it healthy, or names a
// state it will not recover from, or the restore timeout passes. Its duration is the RTO sample.
func (d *drill) waitForTemporary(ctx context.Context, log *slog.Logger) error {
	deadline := d.now().Add(d.cfg.Timeouts.Restore)
	lastPhase := ""
	for {
		found, cluster, err := d.kube.getCluster(ctx, d.cfg.Temporary.Name)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("the temporary cluster disappeared while it was being restored")
		}
		phase, ready := clusterPhase(cluster)
		if phase != lastPhase {
			log.Info("temporary cluster", slog.String("phase", phase))
			lastPhase = phase
		}
		if ready {
			return nil
		}
		if unrecoverable(phase) {
			return fmt.Errorf("the temporary cluster failed: %s", phase)
		}
		if d.now().After(deadline) {
			return fmt.Errorf("the temporary cluster was not ready within %s (last phase: %s)", d.cfg.Timeouts.Restore, phase)
		}
		if err := sleep(ctx, 10*time.Second); err != nil {
			return err
		}
	}
}

// clusterPhase reads the operator's verdict: the phase text, and whether the Ready condition -
// or, on operators without it, the healthy phase - says the cluster serves.
func clusterPhase(cluster map[string]any) (string, bool) {
	status, _ := cluster["status"].(map[string]any)
	phase, _ := status["phase"].(string)
	if conditions, ok := status["conditions"].([]any); ok {
		for _, raw := range conditions {
			condition, _ := raw.(map[string]any)
			if condition["type"] == "Ready" && condition["status"] == "True" {
				return phase, true
			}
		}
	}
	return phase, phase == "Cluster in healthy state"
}

func unrecoverable(phase string) bool {
	lower := strings.ToLower(phase)
	return strings.Contains(lower, "unrecoverable") || strings.Contains(lower, "failed")
}

// verify connects to the restored instance under both live roles and runs the checks.
func (d *drill) verify(ctx context.Context, runID string, liveSchemaVersion int64) ([]check, error) {
	host := d.cfg.Temporary.Name + "-rw"
	ownerDSN, err := rehost(d.cfg.OwnerDSN, host)
	if err != nil {
		return nil, fmt.Errorf("the owner DSN: %w", err)
	}
	appDSN, err := rehost(d.cfg.AppDSN, host)
	if err != nil {
		return nil, fmt.Errorf("the application DSN: %w", err)
	}

	owner, err := d.connect(ctx, ownerDSN)
	if err != nil {
		return nil, fmt.Errorf("the restored database is unreachable as the owner: %w", err)
	}
	defer func() { _ = owner.Close(context.WithoutCancel(ctx)) }()

	app, err := d.connect(ctx, appDSN)
	if err != nil {
		return nil, fmt.Errorf("the restored database is unreachable as the application role: %w", err)
	}
	defer func() { _ = app.Close(context.WithoutCancel(ctx)) }()

	return runChecks(ctx, owner, app, runID, liveSchemaVersion), nil
}

// rehost points a URL-shaped DSN at another host on the standard port. A verifying TLS mode is
// lowered to `require`: the temporary cluster has a certificate authority of its own, and the
// connection stays inside the namespace to a service this program named a minute ago.
func rehost(dsn, host string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("the DSN is not a postgres:// URL")
	}
	parsed.Host = host + ":5432"
	query := parsed.Query()
	switch query.Get("sslmode") {
	case "verify-ca", "verify-full":
		query.Set("sslmode", "require")
	}
	query.Del("sslrootcert")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// teardown deletes the temporary cluster and waits until it is gone, so that the next run - and
// the quota - start from nothing. Absent is a success; `quiet` is for the pre-run sweep, where
// nothing to remove is the expected case and not worth a line.
func (d *drill) teardown(ctx context.Context, log *slog.Logger, quiet bool) error {
	found, _, err := d.kube.getCluster(ctx, d.cfg.Temporary.Name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !quiet {
		log.Info("tearing the temporary cluster down")
	} else {
		log.Warn("a temporary cluster was left behind by an earlier run - removing it")
	}
	waiting, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := d.kube.deleteCluster(waiting, d.cfg.Temporary.Name); err != nil {
		return err
	}
	for {
		found, _, err := d.kube.getCluster(waiting, d.cfg.Temporary.Name)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if err := sleep(waiting, 5*time.Second); err != nil {
			return fmt.Errorf("the temporary cluster is still there: %w", err)
		}
	}
}

// record writes the ConfigMap the gauge reads. Only a pass moves last_success; every run moves
// last_run, so that a failure is visible beside the last success rather than instead of it.
func (d *drill) record(ctx context.Context, ev evidence) error {
	data := map[string]string{
		"last_run": ev.summary(),
	}
	if ev.Result == resultPass {
		data["last_success"] = ev.summary()
		data["last_success_unix"] = strconv.FormatInt(ev.FinishedAt.Unix(), 10)
	}
	if !d.cfg.Evidence.configured() {
		body, err := ev.document()
		if err != nil {
			return err
		}
		data["last_evidence"] = string(body)
	}
	return d.kube.mergeConfigMap(ctx, d.cfg.RecordConfigMap, data)
}

func resultLevel(err error) slog.Level {
	if err != nil {
		return slog.LevelError
	}
	return slog.LevelInfo
}
