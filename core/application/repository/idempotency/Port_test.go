// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package idempotency

import "testing"

// The distinction the guard turns into a 409 rather than a replay: a reserved record with no
// answer yet is a race, not a repeat.
func TestInProgress(t *testing.T) {
	cases := map[string]struct {
		record Record
		want   bool
	}{
		"reserved, not answered": {Record{RequestHash: []byte("h")}, true},
		"answered with 201":      {Record{RequestHash: []byte("h"), Status: 201}, false},
		"answered with 422":      {Record{RequestHash: []byte("h"), Status: 422}, false},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.record.InProgress(); got != c.want {
				t.Errorf("InProgress = %v, want %v", got, c.want)
			}
		})
	}
}

// The endpoint is part of the identity: the same key sent to two operations is two attempts.
func TestTheKeyIsScopedToItsOperation(t *testing.T) {
	first := Key{Key: "k", Endpoint: "POST /api/v1/containers"}
	second := Key{Key: "k", Endpoint: "POST /api/v1/items"}

	if first == second {
		t.Error("one key at two operations is one record")
	}
}
