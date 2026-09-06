// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sealing is the census a rotation ends on (ADR-0045, security.md §8.1): how many stored
// values still name each master key.
//
// Five tables hold a sealed value, and each of their repositories knows how to re-seal its own
// rows; what none of them can answer alone is whether a key may leave the ring. That is one
// question over all five, so it is one port, answered per tenant like everything else and summed
// by the control plane over the tenants it may see.
package sealing

import "context"

// Census counts the sealed values of the current tenant by the key they name.
type Census interface {
	// CountByKey answers, for every key any stored value names, how many name it. A key the ring
	// holds but nothing names is absent from the answer - the caller knows the ring, this does
	// not.
	CountByKey(ctx context.Context) (map[string]int64, error)
}

// Sum adds up the censuses of several workspaces into the installation's. The control plane reads
// each workspace in a bounded transaction of its own and never all of them in one, so what it
// holds is a census per tenant, and this is the one place the arithmetic across them is written.
// A key any workspace names is in the answer; a workspace that names none of a key adds nothing.
func Sum(censuses ...map[string]int64) map[string]int64 {
	total := map[string]int64{}
	for _, census := range censuses {
		for keyID, count := range census {
			total[keyID] += count
		}
	}
	return total
}
