// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sealing is the census a rotation ends on (ADR-0045, security.md §8.1): how many stored
// values still name each master key, and in which kind of value.
//
// Seven stores hold a sealed value, and each of their repositories knows how to re-seal its own
// rows; an eighth, the backup archives, names a key it can never be moved off - an archive is not
// rewritten - and drains only as its archives expire. What none of them can answer alone is
// whether a key may leave the ring. That is one
// question over all of them, so it is one port, answered per tenant like everything else, once
// more for the installation's own rows, and summed by the control plane over the levels it may
// see.
package sealing

import (
	"context"
	"sort"
)

// The stores, a closed set: the census's labels, the contract's enum and the metric's `store`.
const (
	StoreAccountMfa          = "account_mfa"
	StoreIdentityProvider    = "identity_provider"
	StoreWebhookSubscription = "webhook_subscription"
	StoreBackupTarget        = "backup_target"
	StoreAiProvider          = "ai_provider"
	StoreAutomationRule      = "automation_rule"
	// StoreAutomationHTTPJob is an HTTP call's header secret in its job, while the job may still
	// run. It drains by itself as the calls finish; the census counts it so that a key is not
	// retired under a call still on its retry ladder.
	StoreAutomationHTTPJob = "automation_http_job"
	// StoreBackupArchive is an archive this installation wrote and still keeps, under the master
	// key its manifest names (backup-restore.md §4). Re-sealing cannot move it; it drains as the
	// archives expire, and until then retiring the key would leave them unreadable.
	StoreBackupArchive = "backup_archive"
)

// Stores is the closed set as one list, which a test holds to the contract's enum and to the
// messages that name each store.
var Stores = []string{
	StoreAccountMfa, StoreIdentityProvider, StoreWebhookSubscription, StoreBackupTarget,
	StoreAiProvider, StoreAutomationRule, StoreAutomationHTTPJob, StoreBackupArchive,
}

// Counts is a census: for every key a stored value names, how many values of each store name it.
// A key the ring holds but nothing names is absent - the caller knows the ring, this does not.
type Counts map[string]map[string]int64

// Add records n values of one store under one key.
func (c Counts) Add(keyID, store string, n int64) {
	if n == 0 {
		return
	}
	if c[keyID] == nil {
		c[keyID] = map[string]int64{}
	}
	c[keyID][store] += n
}

// ByKey answers the total per key, over every store.
func (c Counts) ByKey() map[string]int64 {
	totals := make(map[string]int64, len(c))
	for keyID, stores := range c {
		for _, n := range stores {
			totals[keyID] += n
		}
	}
	return totals
}

// StoreCount is one store's share of a key.
type StoreCount struct {
	Store string
	Count int64
}

// Stores answers one key's stores in a stable order, the ones with nothing left out.
func (c Counts) Stores(keyID string) []StoreCount {
	stores := make([]StoreCount, 0, len(c[keyID]))
	for store, n := range c[keyID] {
		if n > 0 {
			stores = append(stores, StoreCount{Store: store, Count: n})
		}
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].Store < stores[j].Store })
	return stores
}

// Census counts sealed values by key and store, at the level of the transaction's scope.
type Census interface {
	// Count answers the current tenant's own values. A row of the installation's that the read
	// policy shows the workspace is not the workspace's, and is not in the answer.
	Count(ctx context.Context) (Counts, error)
	// CountInstallation answers the installation's own values - the sign-in providers and AI
	// models it offers - read in the installation's scope.
	CountInstallation(ctx context.Context) (Counts, error)
}

// Sum adds up the censuses of several levels into the installation's. The control plane reads
// each workspace in a bounded transaction of its own and never all of them in one, so what it
// holds is a census per level, and this is the one place the arithmetic across them is written.
func Sum(censuses ...Counts) Counts {
	total := Counts{}
	for _, census := range censuses {
		for keyID, stores := range census {
			for store, n := range stores {
				total.Add(keyID, store, n)
			}
		}
	}
	return total
}
