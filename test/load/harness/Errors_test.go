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
