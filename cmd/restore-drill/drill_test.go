// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRehostPointsADSNAtTheTemporaryCluster(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"plain": {
			"postgres://hubtask_owner:pw@db-rw:5432/hubtask",
			"postgres://hubtask_owner:pw@db-drill-rw:5432/hubtask",
		},
		"a verifying mode is lowered and its root certificate dropped": {
			"postgresql://app:pw@db-rw:5432/hubtask?sslmode=verify-full&sslrootcert=/ca.crt",
			"postgresql://app:pw@db-drill-rw:5432/hubtask?sslmode=require",
		},
		"require stays": {
			"postgres://app:pw@db-rw/hubtask?sslmode=require",
			"postgres://app:pw@db-drill-rw:5432/hubtask?sslmode=require",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := rehost(test.in, "db-drill-rw")
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
	if _, err := rehost("host=db user=app", "x"); err == nil {
		t.Error("a key/value DSN was accepted")
	}
}

func TestSegmentArchivedComparesNamesInTheirOrder(t *testing.T) {
	wanted := "000000010000000000000042"
	if !segmentArchived(wanted, wanted) {
		t.Error("the segment itself does not count as archived")
	}
	if !segmentArchived("000000010000000000000043", wanted) {
		t.Error("a later segment does not count")
	}
	if segmentArchived("000000010000000000000041", wanted) {
		t.Error("an earlier segment counts")
	}
	if !segmentArchived("000000020000000000000001", wanted) {
		t.Error("a later timeline does not count")
	}
	if segmentArchived("not-a-segment", wanted) {
		t.Error("garbage counts")
	}
}

func TestTheTemporaryClusterRecoversToTheTargetWithoutABackupStanza(t *testing.T) {
	cfg, err := load(t, complete(), nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg.Archive.EndpointURL = "https://s3.example.com"
	cfg.Temporary.ResourcesJSON = `{"requests":{"cpu":"500m"}}`
	target := time.Date(2026, 9, 7, 10, 4, 5, 123456000, time.FixedZone("CEST", 2*3600))

	manifest := temporaryClusterManifest(cfg, target)
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Backup    *struct{} `json:"backup"`
			Resources struct {
				Requests map[string]string `json:"requests"`
			} `json:"resources"`
			Bootstrap struct {
				Recovery struct {
					Source         string                `json:"source"`
					Owner          string                `json:"owner"`
					Secret         struct{ Name string } `json:"secret"`
					RecoveryTarget struct {
						TargetTime string `json:"targetTime"`
					} `json:"recoveryTarget"`
				} `json:"recovery"`
			} `json:"bootstrap"`
			External []struct {
				Name  string `json:"name"`
				Store struct {
					DestinationPath string `json:"destinationPath"`
					EndpointURL     string `json:"endpointURL"`
					ServerName      string `json:"serverName"`
				} `json:"barmanObjectStore"`
			} `json:"externalClusters"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Metadata.Name != "db-drill" || decoded.Metadata.Namespace != "workspace" {
		t.Errorf("named %s in %s", decoded.Metadata.Name, decoded.Metadata.Namespace)
	}
	if decoded.Spec.Backup != nil {
		t.Error("the temporary cluster carries a backup stanza - it would archive over the live archive")
	}
	if got := decoded.Spec.Bootstrap.Recovery.RecoveryTarget.TargetTime; got != "2026-09-07 08:04:05.123456+00" {
		t.Errorf("the target is %q, want it in UTC with microseconds", got)
	}
	if decoded.Spec.Bootstrap.Recovery.Source != "live" || decoded.Spec.External[0].Name != "live" {
		t.Error("the recovery source and the external cluster do not name each other")
	}
	if decoded.Spec.Bootstrap.Recovery.Secret.Name != "db-app" || decoded.Spec.Bootstrap.Recovery.Owner != "hubtask_owner" {
		t.Error("the recovered owner does not get the live cluster's credential")
	}
	store := decoded.Spec.External[0].Store
	if store.DestinationPath != "s3://backups/hubtask" || store.EndpointURL != "https://s3.example.com" || store.ServerName != "db" {
		t.Errorf("the archive is read from %+v", store)
	}
	if decoded.Spec.Resources.Requests["cpu"] != "500m" {
		t.Error("the resources stanza was not embedded verbatim")
	}
}

func TestClusterPhaseReadsTheOperatorsVerdict(t *testing.T) {
	healthy := map[string]any{"status": map[string]any{"phase": "Cluster in healthy state"}}
	if _, ready := clusterPhase(healthy); !ready {
		t.Error("the healthy phase is not ready")
	}
	byCondition := map[string]any{"status": map[string]any{
		"phase":      "Waiting for the instances to become active",
		"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
	}}
	if _, ready := clusterPhase(byCondition); !ready {
		t.Error("a Ready condition is not ready")
	}
	starting := map[string]any{"status": map[string]any{"phase": "Setting up primary"}}
	if phase, ready := clusterPhase(starting); ready || phase != "Setting up primary" {
		t.Errorf("starting reads as ready=%t phase=%q", ready, phase)
	}
	if _, ready := clusterPhase(map[string]any{}); ready {
		t.Error("no status reads as ready")
	}
	if !unrecoverable("Cluster is in an unrecoverable state, needs manual intervention") || unrecoverable("Creating primary") {
		t.Error("the unrecoverable phases are misjudged")
	}
}

func TestRecoverabilityPointIsReadFromTheStatus(t *testing.T) {
	if _, ok := recoverabilityPoint(map[string]any{"status": map[string]any{}}); ok {
		t.Error("no point reads as a point")
	}
	point, ok := recoverabilityPoint(map[string]any{"status": map[string]any{
		"firstRecoverabilityPoint": "2026-09-07T02:31:00Z",
	}})
	if !ok || !point.Equal(time.Date(2026, 9, 7, 2, 31, 0, 0, time.UTC)) {
		t.Errorf("read %v %t", point, ok)
	}
}
