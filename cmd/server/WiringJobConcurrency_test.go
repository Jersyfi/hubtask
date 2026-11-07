// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
)

// The jobs running at once stay within what the background pool serves beside its held
// connections, and a pool too small for the setting runs fewer rather than refusing the start.
func TestTheJobsRunningAtOnceFitTheBackgroundPool(t *testing.T) {
	cases := []struct {
		name               string
		concurrency, conns int
		want               int
	}{
		{"the default within the default pool", 4, 10, 4},
		{"a setting the pool cannot serve", 20, 10, 7},
		{"a pool of the held connections alone", 4, 3, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cfg envport.Config
			cfg.Queue.Concurrency, cfg.Database.MaxConns = c.concurrency, c.conns
			if got := jobConcurrency(t.Context(), cfg); got != c.want {
				t.Errorf("jobConcurrency = %d, want %d", got, c.want)
			}
		})
	}
}
