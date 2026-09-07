// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func complete() map[string]string {
	return map[string]string{
		"HUBTASK_DB_DSN":                         "postgres://owner:pw@db-rw:5432/hubtask",
		"HUBTASK_DRILL_APP_DSN":                  "postgres://hubtask_app:pw@db-rw:5432/hubtask",
		"HUBTASK_DRILL_NAMESPACE":                "workspace",
		"HUBTASK_DRILL_SOURCE_CLUSTER":           "db",
		"HUBTASK_DRILL_ARCHIVE_DESTINATION_PATH": "s3://backups/hubtask",
		"HUBTASK_DRILL_ARCHIVE_SECRET":           "backup-s3",
		"HUBTASK_DRILL_IMAGE_NAME":               "ghcr.io/cloudnative-pg/postgresql:17.6",
		"HUBTASK_DRILL_STORAGE_SIZE":             "8Gi",
		"HUBTASK_DRILL_RECORD_CONFIGMAP":         "hubtask-restore-drill",
	}
}

func load(t *testing.T, values map[string]string, files map[string]string) (config, error) {
	t.Helper()
	get := func(name string) string { return values[name] }
	read := func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, errors.New("no such file")
		}
		return []byte(content), nil
	}
	return loadConfig(get, read)
}

func TestACompleteConfigurationLoadsWithItsDefaults(t *testing.T) {
	cfg, err := load(t, complete(), nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if cfg.Temporary.Name != "db-drill" || cfg.Temporary.OwnerSecret != "db-app" || cfg.Archive.ServerName != "db" {
		t.Errorf("the derived names are %q, %q, %q", cfg.Temporary.Name, cfg.Temporary.OwnerSecret, cfg.Archive.ServerName)
	}
	if cfg.Timeouts.Total != 45*time.Minute || cfg.MarkerRetention != 35*24*time.Hour {
		t.Errorf("defaults: total %s, retention %s", cfg.Timeouts.Total, cfg.MarkerRetention)
	}
	if cfg.Evidence.configured() {
		t.Error("no bucket was named and the evidence is configured")
	}
}

func TestEveryMissingValueIsNamedAtOnce(t *testing.T) {
	_, err := load(t, map[string]string{"HUBTASK_DRILL_NAMESPACE": "workspace"}, nil)
	if err == nil {
		t.Fatal("an empty environment was accepted")
	}
	for _, name := range []string{
		"HUBTASK_DB_DSN", "HUBTASK_DRILL_APP_DSN", "HUBTASK_DRILL_SOURCE_CLUSTER",
		"HUBTASK_DRILL_ARCHIVE_DESTINATION_PATH", "HUBTASK_DRILL_ARCHIVE_SECRET",
		"HUBTASK_DRILL_IMAGE_NAME", "HUBTASK_DRILL_STORAGE_SIZE", "HUBTASK_DRILL_RECORD_CONFIGMAP",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s is missing and the error does not say so: %v", name, err)
		}
	}
}

func TestTheNamespaceComesFromThePodWhenNotSet(t *testing.T) {
	values := complete()
	delete(values, "HUBTASK_DRILL_NAMESPACE")
	cfg, err := load(t, values, map[string]string{defaultNamespaceFile: "from-the-pod\n"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if cfg.Namespace != "from-the-pod" {
		t.Errorf("namespace %q", cfg.Namespace)
	}
	if _, err := load(t, values, nil); err == nil {
		t.Error("no namespace anywhere was accepted")
	}
}

func TestASecretMayArriveAsAFile(t *testing.T) {
	values := complete()
	delete(values, "HUBTASK_DB_DSN")
	values["HUBTASK_DB_DSN_FILE"] = "/run/secrets/dsn"
	cfg, err := load(t, values, map[string]string{"/run/secrets/dsn": "postgres://o:p@h:5432/d\n"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if cfg.OwnerDSN != "postgres://o:p@h:5432/d" {
		t.Errorf("the DSN was read as %q", cfg.OwnerDSN)
	}
}

func TestAnEvidenceBucketNeedsItsKeys(t *testing.T) {
	values := complete()
	values["HUBTASK_DRILL_EVIDENCE_BUCKET"] = "evidence"
	if _, err := load(t, values, nil); err == nil || !strings.Contains(err.Error(), "ACCESS_KEY") {
		t.Errorf("a bucket without keys: %v", err)
	}
	values["HUBTASK_DRILL_EVIDENCE_ACCESS_KEY"] = "k"
	values["HUBTASK_DRILL_EVIDENCE_SECRET_KEY"] = "s"
	values["HUBTASK_DRILL_EVIDENCE_PREFIX"] = "/prod/"
	cfg, err := load(t, values, nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if key := cfg.Evidence.evidenceKey("run-1"); key != "prod/restore-drill/run-1.json" {
		t.Errorf("evidence key %q", key)
	}
}

func TestTheTemporaryClusterMayNotBeTheSource(t *testing.T) {
	values := complete()
	values["HUBTASK_DRILL_TEMPORARY_CLUSTER"] = "db"
	if _, err := load(t, values, nil); err == nil {
		t.Error("restoring over the source was accepted")
	}
}

func TestABadDurationIsRefusedRatherThanDefaulted(t *testing.T) {
	values := complete()
	values["HUBTASK_DRILL_TIMEOUT"] = "later"
	if _, err := load(t, values, nil); err == nil || !strings.Contains(err.Error(), "HUBTASK_DRILL_TIMEOUT") {
		t.Errorf("a bad duration: %v", err)
	}
}
