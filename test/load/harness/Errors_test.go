// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"strings"
	"testing"
)

func TestServerErrorsAreCountedPerMessage(t *testing.T) {
	log := strings.Join([]string{
		`{"time":"t","level":"INFO","msg":"ready"}`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":812}`,
		`=== RUN   TestCapacityAtScale`,
		`{"time":"t","level":"WARN","msg":"a slow query","ms":907}`,
		`{"time":"t","level":"ERROR","msg":"the pool is exhausted"}`,
	}, "\n")
	got, err := ServerErrors(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (LogError{Source: "server", Level: "WARN", Message: "a slow query", Count: 2}) ||
		got[1].Level != "ERROR" || got[1].Count != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestPostgresErrorsAreCountedWithTheirValuesMasked(t *testing.T) {
	log := strings.Join([]string{
		`2026-10-31 13:00:00.000 UTC [61] LOG:  checkpoint starting: time`,
		`2026-10-31 13:00:01.000 UTC [62] ERROR:  deadlock detected on relation 16384`,
		`2026-10-31 13:00:02.000 UTC [63] ERROR:  deadlock detected on relation 16999`,
		`2026-10-31 13:00:03.000 UTC [64] FATAL:  terminating connection due to administrator command`,
	}, "\n")
	got, err := PostgresErrors(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Count != 2 || got[0].Message != "deadlock detected on relation …" || got[1].Level != "FATAL" {
		t.Errorf("got %+v", got)
	}
}

// The migration tool's first look at an empty database is its own probe, not the product's error;
// any other missing relation still counts.
func TestTheMigrationToolsFirstProbeIsNotAnError(t *testing.T) {
	log := strings.Join([]string{
		`2026-10-31 23:52:03.706 UTC [79] ERROR:  relation "goose_db_version" does not exist at character 36`,
		`2026-10-31 23:52:04.706 UTC [80] ERROR:  relation "work_item" does not exist at character 15`,
	}, "\n")
	got, err := PostgresErrors(strings.NewReader(log))
	if err != nil || len(got) != 1 || got[0].Count != 1 {
		t.Fatalf("rows %+v (%v)", got, err)
	}
}
