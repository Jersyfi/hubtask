// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// samlWatchInterval bounds how long the watch sleeps: a certificate pasted by a build that does
// not seed the watch is still seen within a day.
const samlWatchInterval = 24 * time.Hour

// SAMLExpiryNotice tells the workspace's administrators that a SAML provider's certificates end.
type SAMLExpiryNotice interface {
	SAMLCertificateEnding(ctx context.Context, tenantID, providerID shared.ID) error
}

// WatchSAMLCertificates tells a workspace's administrators 30, 7 and 1 days before the last
// certificate of one of its SAML providers ends (identity.md §26.1): from then on every answer is
// refused and nobody signs in through it.
type WatchSAMLCertificates struct {
	Watches    repository.SAMLCertificateWatches
	Tell       SAMLExpiryNotice
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute gives the warnings owed and answers when to look again, nil when the workspace has no
// SAML provider - the next one configured seeds the watch again.
func (w WatchSAMLCertificates) Execute(ctx context.Context, actor appshared.ActorContext) (*time.Time, error) {
	now := w.Clock.Now()
	next := now.Add(samlWatchInterval)
	found := false
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		watches, err := w.Watches.SAMLWatches(ctx)
		if err != nil {
			return err
		}
		found = len(watches) > 0
		for _, watch := range watches {
			days, at := watch.Owed(now)
			if !at.IsZero() && at.Before(next) {
				next = at
			}
			if days == 0 {
				continue
			}
			end, _ := watch.CertificatesEnd()
			if err := w.Tell.SAMLCertificateEnding(ctx, actor.TenantID, watch.ProviderID); err != nil {
				return err
			}
			if err := w.Watches.MarkSAMLExpiryTold(ctx, watch.ProviderID, end, days); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || !found {
		return nil, err
	}
	return &next, nil
}

// SeedSAMLCertificateWatch makes sure the workspace's watch runs, inside the caller's transaction.
// The dedupe key is the workspace, so a second provider joins the watch already there.
func SeedSAMLCertificateWatch(ctx context.Context, jobs SessionJobs, tenantID shared.ID, now time.Time) error {
	if jobs == nil {
		return nil
	}
	_, err := jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindSAMLCertificateWatch, TenantID: tenantID,
		DedupeKey: "saml-certificates:" + tenantID.String(), RunAt: now,
	})
	return err
}
