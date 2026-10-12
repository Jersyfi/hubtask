// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

const (
	// KeyMissingCode is a key the ring no longer holds while values of one kind still name it -
	// the health report's answer to UC-INS-16 check 3: which kind would stay unreadable.
	KeyMissingCode = "crypto.key_missing"
	// KeyCensusUncheckedCode says the count behind the warning above could not be taken, or not
	// yet: never read as "nothing is missing" (P-11).
	KeyCensusUncheckedCode = "crypto.key_census_unchecked"

	keyHealthComponent = "sealing.key_health"
	// neverCounted is the moment of a count that has not been taken; the message says nothing then.
	neverCounted = "none"
	// KeyHealthEvery is how old a census may be before a report asks for a new one. Hourly,
	// because the count crosses every workspace and a health report is read far more often than
	// a key leaves the ring.
	KeyHealthEvery = time.Hour
	// KeyHealthTimeout bounds one census for the report (rule 7).
	KeyHealthTimeout = 60 * time.Second
)

// KeyHealth is the health report's view of the keyring: the census of every level, taken in the
// background and served from the last one, never on the report's own path - a census is one
// transaction per workspace, and a report must answer at once.
//
// What a report shows says when it was counted (`counted_at`), so a stale number is never shown as
// current; before the first count, or when the last one failed, it says the count is unchecked.
type KeyHealth struct {
	Tenants    adminrepo.Tenants
	Census     sealingrepo.Census
	Encryptor  cryptoport.Encryptor
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock

	mu        sync.Mutex
	counted   bool
	failed    bool
	running   bool
	countedAt time.Time
	counts    sealingrepo.Counts
}

// Warnings is the health report's source: the last census's warnings, and a new census started
// when the last one is older than KeyHealthEvery.
func (k *KeyHealth) Warnings(ctx context.Context) []health.Warning {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.running && (!k.counted || k.Clock.Now().Sub(k.countedAt) >= KeyHealthEvery) {
		k.startLocked(ctx)
	}
	if !k.counted {
		severity := "info"
		if k.failed {
			severity = "warn"
		}
		return []health.Warning{{
			Code: KeyCensusUncheckedCode, Severity: severity,
			Params: map[string]string{"counted_at": neverCounted},
		}}
	}
	warnings := missingKeys(k.counts, k.Encryptor.KeyIDs(), k.countedAt)
	if k.failed {
		// The last attempt failed: the older count is still shown, said to be as old as it is.
		warnings = append(warnings, health.Warning{
			Code: KeyCensusUncheckedCode, Severity: "warn",
			Params: map[string]string{"counted_at": k.countedAt.UTC().Format(time.RFC3339)},
		})
	}
	return warnings
}

// Refresh starts a census now, unless one is running. Called at start.
func (k *KeyHealth) Refresh(ctx context.Context) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.running {
		k.startLocked(ctx)
	}
}

// Observe takes a census somebody else has just counted - the operator's read of the encryption
// status - so the report is as fresh as the screen the operator is looking at.
func (k *KeyHealth) Observe(counts sealingrepo.Counts) {
	at := k.Clock.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	k.counted, k.failed, k.counts, k.countedAt = true, false, counts, at
}

// startLocked runs one census in the background, detached from the report that asked for it and
// bounded by its own deadline.
func (k *KeyHealth) startLocked(ctx context.Context) {
	k.running = true
	concurrency.Go(context.WithoutCancel(ctx), keyHealthComponent, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, KeyHealthTimeout)
		defer cancel()
		counts, err := k.count(ctx)
		at := k.Clock.Now()

		k.mu.Lock()
		defer k.mu.Unlock()
		k.running = false
		if err != nil {
			k.failed = true
			return
		}
		k.counted, k.failed, k.counts, k.countedAt = true, false, counts, at
	})
}

func (k *KeyHealth) count(ctx context.Context) (sealingrepo.Counts, error) {
	var tenants []adminrepo.TenantRecord
	err := k.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			listed, err := k.Tenants.List(ctx)
			tenants = listed
			return err
		})
	if err != nil {
		return nil, err
	}
	return Count(ctx, k.UnitOfWork, k.Census, tenants)
}

// missingKeys is one warning per key the ring lacks and kind still sealed under it. Counts and
// kinds only - never a workspace (rule 10).
func missingKeys(counts sealingrepo.Counts, ring []string, at time.Time) []health.Warning {
	held := map[string]bool{}
	for _, keyID := range ring {
		held[keyID] = true
	}
	missing := make([]string, 0)
	for keyID := range counts {
		if !held[keyID] {
			missing = append(missing, keyID)
		}
	}
	sort.Strings(missing)
	warnings := make([]health.Warning, 0, len(missing))
	for _, keyID := range missing {
		for _, store := range counts.Stores(keyID) {
			warnings = append(warnings, health.Warning{
				Code: KeyMissingCode, Severity: "warn",
				Params: map[string]string{
					"key": keyID, "store": store.Store,
					"count":      strconv.FormatInt(store.Count, 10),
					"counted_at": at.UTC().Format(time.RFC3339),
				},
			})
		}
	}
	return warnings
}
