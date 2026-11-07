// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"strings"
	"testing"
	"time"
)

// S3 counts a snapshot's records and bytes and tells one read whole - the cursor last - from one
// cut short (test/load/PLAN.md, S3).
func TestASnapshotIsWholeOnlyWithItsCursorLast(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		records int
		whole   bool
		bad     int
	}{
		{name: "whole", body: "{\"op\":\"UPSERT\"}\n{\"op\":\"UPSERT\"}\n{\"cursor\":\"c1\"}\n", records: 2, whole: true},
		{name: "cut short", body: "{\"op\":\"UPSERT\"}\n{\"op\":\"UPS", records: 1, bad: 1},
		{name: "a record after the cursor", body: "{\"cursor\":\"c1\"}\n{\"op\":\"UPSERT\"}\n", records: 1},
		{name: "empty", body: ""},
		{name: "an empty cursor", body: "{\"cursor\":\"\"}\n"},
	}
	for _, c := range cases {
		got, err := ReadSnapshot(strings.NewReader(c.body), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if got.Records != c.records || got.CursorReceived != c.whole || got.Malformed != c.bad ||
			got.Bytes < int64(len(c.body)) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}
