// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"log/slog"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/dns"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	// OwnDomainCheckInterval is how often the check comes back. A pending domain is looked at on
	// every pass - its owner is waiting - and a serving one once an hour.
	OwnDomainCheckInterval  = 5 * time.Minute
	ownDomainServingRecheck = time.Hour
	// ownDomainCheckBatch bounds one pass; a backlog is worked off over several.
	ownDomainCheckBatch = 200
	ownDomainCheckKey   = "installation"
)

// CheckOwnDomains is the job behind verification (UC-INS-15 check 3): it looks up each due domain's
// two records through the DNS port, outside any transaction, and moves the row in its workspace's
// own transaction - pending to verified, serving to broken, broken back - with the move in the
// workspace's trail. A main domain that breaks hands the main address back to the subdomain in the
// same transaction. A lookup that got no answer moves nothing.
type CheckOwnDomains struct {
	OwnDomains
	Resolver dns.Resolver
	Jobs     JobQueue
}

// Seed asks for the check once a worker starts. Single mode has no own domains and seeds nothing.
func (c CheckOwnDomains) Seed(ctx context.Context) error {
	if c.Tenancy != env.TenancyMulti {
		return nil
	}
	return c.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := c.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindOwnDomainCheck, DedupeKey: ownDomainCheckKey, RunAt: c.Clock.Now().UTC(),
		})
		return err
	})
}

// Run does one pass and answers how many domains changed state.
func (c CheckOwnDomains) Run(ctx context.Context) (int, error) {
	now := c.Clock.Now()
	var due []identityrepo.DueOwnDomain
	err := c.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		rows, err := c.Doors.Due(ctx, now.Add(-OwnDomainCheckInterval+time.Minute), now.Add(-ownDomainServingRecheck), ownDomainCheckBatch)
		due = rows
		return err
	})
	if err != nil || len(due) == 0 {
		return 0, err
	}
	target := domain.OwnDomainTarget(c.InstallationHost)
	targetCanonical, err := c.Resolver.CanonicalName(ctx, target)
	if err != nil {
		// Without the target's own chain a flattened record is still compared with the target.
		targetCanonical = ""
	}
	moved := 0
	var failed error
	for _, row := range due {
		// One workspace's failing write does not hold up the others; the pass is retried for it.
		changed, err := c.record(ctx, row, c.look(ctx, row.Host, target, targetCanonical))
		if err != nil {
			failed = err
			continue
		}
		if changed {
			moved++
		}
	}
	return moved, failed
}

// look asks the DNS for one domain's two records; the proof is the mark verbatim. The mark is not
// read from the row here: record compares it with the row it holds in its transaction.
func (c CheckOwnDomains) look(ctx context.Context, host, target, targetCanonical string) lookAt {
	texts, err := c.Resolver.TXT(ctx, domain.OwnDomainProofName(host))
	if err != nil {
		return lookAt{}
	}
	canonical, err := c.Resolver.CanonicalName(ctx, host)
	if err != nil {
		return lookAt{}
	}
	return lookAt{known: true, texts: texts, targetFound: domain.TargetReached(canonical, target, targetCanonical)}
}

type lookAt struct {
	known       bool
	texts       []string
	targetFound bool
}

// record moves one row in its workspace's transaction and writes the move to the trail.
func (c CheckOwnDomains) record(ctx context.Context, due identityrepo.DueOwnDomain, at lookAt) (bool, error) {
	changed := false
	err := c.UnitOfWork.Within(ctx, persistence.Scope{TenantID: due.TenantID}, func(ctx context.Context) error {
		current, err := c.Domains.Find(ctx)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.Host != due.Host {
			return nil
		}
		now := c.Clock.Now()
		look := domain.DomainLookup{
			Known: at.known, TargetFound: at.targetFound,
			ProofFound: domain.ProofCarried(at.texts, current.Verification),
		}
		if !look.Known {
			return nil
		}
		next := domain.NextOwnDomainState(current.State, look, current.CertificateAsked)
		written, err := c.Domains.RecordLook(ctx, current.Host, current.State, next, look, now)
		if err != nil || !written || next == current.State {
			return err
		}
		changed = true
		action, severity := OwnDomainVerifiedAction, audit.SeverityInfo
		changes := []audit.Change{{Field: "state", Classification: audit.Open, From: string(current.State), To: string(next)}}
		if next == domain.HostBroken {
			action, severity = OwnDomainBrokenAction, audit.SeverityWarning
			// A main domain that broke hands the main address back in this same transaction
			// (ADR-0095 §4, UC-INS-15 check 9); the moment stays for the warning.
			if current.Main {
				to, err := c.Domains.MainToAddress(ctx, now)
				if err != nil {
					return err
				}
				changes = append(changes, audit.Change{Field: "host", Classification: audit.Open, From: current.Host, To: to})
			}
		}
		return c.trail(ctx, due.TenantID, appshared.ActorContext{}, action, severity, changes[0], now, changes[1:]...)
	})
	if err != nil {
		slog.WarnContext(ctx, "an own domain's look was not recorded", slog.String("error", err.Error()))
		return false, err
	}
	return changed, nil
}
