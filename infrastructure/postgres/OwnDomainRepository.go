// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// OwnDomainRepository is a workspace's own domain and the operator's blocks (migration 0211).
// The workspace's statements are bounded by row level security; the ones across workspaces go
// through the SECURITY DEFINER doors, callable in any scope.
type OwnDomainRepository struct{}

func NewOwnDomainRepository() OwnDomainRepository { return OwnDomainRepository{} }

var (
	_ repository.OwnDomains     = OwnDomainRepository{}
	_ repository.OwnDomainDoors = OwnDomainRepository{}
	_ repository.BrowserHosts   = OwnDomainRepository{}
	_ repository.MainAddresses  = OwnDomainRepository{}
)

func ownDomainFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func timestamp(at time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: at, Valid: true} }

func (OwnDomainRepository) Find(ctx context.Context) (identity.OwnDomain, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OwnDomain{}, err
	}
	row, err := queries.FindOwnDomain(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.OwnDomain{}, shared.ErrNotFound.WithDetail("admin.own_domain_none")
	}
	if err != nil {
		return identity.OwnDomain{}, ownDomainFailed("reading the own domain", err)
	}
	since := row.StateSince.Time
	if !row.StateSince.Valid {
		since = row.CreatedAt.Time
	}
	return identity.OwnDomain{
		Host: row.Host, State: identity.HostState(row.State), Verification: row.Verification,
		Since: since, CheckedAt: row.CheckedAt.Time, TargetFound: row.TargetFound, ProofFound: row.ProofFound,
		CertificateAsked: row.CertificateAsked, Main: row.IsCanonical, FellBackAt: row.FellBackAt.Time,
	}, nil
}

func (OwnDomainRepository) Insert(ctx context.Context, host identity.TenantHost) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	err = queries.InsertOwnDomain(ctx, sqlc.InsertOwnDomainParams{
		Host: host.Host, Verification: host.Verification, Now: timestamp(host.CreatedAt),
	})
	if isUniqueViolation(err) {
		// The host index says another workspace answers at it; the one-per-workspace index cannot
		// fire here, the use case read the workspace's row first. Whose it is stays unsaid (P-01).
		return identity.OwnDomainTaken(host.Host)
	}
	if err != nil {
		return ownDomainFailed("writing the own domain", err)
	}
	return nil
}

func (OwnDomainRepository) Remove(ctx context.Context) (identity.OwnDomain, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.OwnDomain{}, err
	}
	row, err := queries.DeleteOwnDomain(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.OwnDomain{}, shared.ErrNotFound.WithDetail("admin.own_domain_none")
	}
	if err != nil {
		return identity.OwnDomain{}, ownDomainFailed("removing the own domain", err)
	}
	return identity.OwnDomain{Host: row.Host, Main: row.IsCanonical}, nil
}

func (OwnDomainRepository) Blocked(ctx context.Context, host string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	blocked, err := queries.OwnDomainIsBlocked(ctx, host)
	if err != nil {
		return false, ownDomainFailed("reading the blocks", err)
	}
	return blocked, nil
}

func (OwnDomainRepository) BlockInView(ctx context.Context) (identity.DomainBlock, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.DomainBlock{}, err
	}
	row, err := queries.BlockInView(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.DomainBlock{}, shared.ErrNotFound.WithDetail("admin.own_domain_none")
	}
	if err != nil {
		return identity.DomainBlock{}, ownDomainFailed("reading the workspace's block", err)
	}
	return identity.DomainBlock{Host: row.Host, Reason: row.Reason, BlockedAt: row.BlockedAt.Time, WasMain: row.WasMain}, nil
}

func (OwnDomainRepository) DismissBlocks(ctx context.Context, now time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	dismissed, err := queries.DismissBlocks(ctx, timestamp(now))
	if err != nil {
		return 0, ownDomainFailed("dismissing the workspace's blocks", err)
	}
	return int(dismissed), nil
}

func (OwnDomainRepository) Block(ctx context.Context, block identity.DomainBlock) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	err = queries.InsertBlockedDomain(ctx, sqlc.InsertBlockedDomainParams{
		Host: block.Host, Reason: block.Reason, BlockedAt: timestamp(block.BlockedAt), WasMain: block.WasMain,
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("admin.own_domain_already_blocked")
	}
	if err != nil {
		return ownDomainFailed("keeping the block", err)
	}
	return nil
}

func (OwnDomainRepository) RecordLook(
	ctx context.Context, host string, from, next identity.HostState, look identity.DomainLookup, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	written, err := queries.RecordOwnDomainLook(ctx, sqlc.RecordOwnDomainLookParams{
		NextState: string(next), Now: timestamp(now), TargetFound: look.TargetFound,
		ProofFound: look.ProofFound, Host: host, FromState: string(from),
	})
	if err != nil {
		return false, ownDomainFailed("recording a look at the own domain", err)
	}
	return written == 1, nil
}

func (OwnDomainRepository) Due(
	ctx context.Context, pendingBefore, servingBefore time.Time, most int,
) ([]repository.DueOwnDomain, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.OwnDomainsDue(ctx, sqlc.OwnDomainsDueParams{
		PendingBefore: timestamp(pendingBefore), ServingBefore: timestamp(servingBefore),
		Most: int32(most), //nolint:gosec // G115: a batch size, bounded small by the check
	})
	if err != nil {
		return nil, ownDomainFailed("reading the own domains due", err)
	}
	due := make([]repository.DueOwnDomain, 0, len(rows))
	for _, row := range rows {
		tenant, err := idFrom(row.TenantID)
		if err != nil {
			return nil, err
		}
		due = append(due, repository.DueOwnDomain{TenantID: tenant, Host: row.Host})
	}
	return due, nil
}

func (OwnDomainRepository) All(ctx context.Context) ([]repository.ListedOwnDomain, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.AdminOwnDomains(ctx)
	if err != nil {
		return nil, ownDomainFailed("reading the own domains", err)
	}
	listed := make([]repository.ListedOwnDomain, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, repository.ListedOwnDomain{
			Host: row.Host, TenantID: shared.ID(row.TenantID), TenantName: row.TenantName,
			State: identity.HostState(row.State), Since: row.Since.Time, Main: row.IsMain,
			BlockedReason: row.BlockedReason, FellBackAt: row.FellBackAt.Time, AddressHost: row.AddressHost,
		})
	}
	return listed, nil
}

func (OwnDomainRepository) Certificate(ctx context.Context, host string, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	allowed, err := queries.OwnDomainCertificate(ctx, sqlc.OwnDomainCertificateParams{Host: host, AskedAt: timestamp(at)})
	if err != nil {
		return false, ownDomainFailed("answering the ingress", err)
	}
	return allowed, nil
}

var _ repository.ServingHosts = SignInRepository{}

// ResolveServing answers the workspace a host names and whether it awaits its first request, in
// one round trip (migration 0211).
func (r SignInRepository) ResolveServing(ctx context.Context, host string) (shared.ID, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", false, err
	}
	row, err := queries.ResolveServingHost(ctx, host)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
	}
	if err != nil {
		return "", false, ownDomainFailed("resolving the host", err)
	}
	tenant, err := idFrom(row.TenantID)
	return tenant, row.AwaitsRequest, err
}

// Served makes a verified own domain the ingress asked about active. The moment is this process's:
// the request path holds no clock, and it is shown, never compared.
func (r SignInRepository) Served(ctx context.Context, host string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	moved, err := queries.OwnDomainServed(ctx, sqlc.OwnDomainServedParams{Host: host, ServedAt: timestamp(time.Now().UTC())})
	if err != nil {
		return false, ownDomainFailed("recording the own domain's first request", err)
	}
	return moved, nil
}

func (OwnDomainRepository) AddressHost(ctx context.Context) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	host, err := queries.AddressHost(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", ownDomainFailed("reading the workspace's address", err)
	}
	return host, nil
}

func (OwnDomainRepository) MainToOwnDomain(ctx context.Context) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	cleared, err := queries.ClearMainHost(ctx)
	if err != nil {
		return "", ownDomainFailed("clearing the main address", err)
	}
	made, err := queries.MakeOwnDomainMain(ctx)
	if err != nil {
		return "", ownDomainFailed("making the own domain the main address", err)
	}
	if made == 0 {
		return "", shared.ErrConflict.WithDetail("admin.own_domain_not_active")
	}
	if len(cleared) == 0 {
		return "", nil
	}
	return cleared[0], nil
}

func (OwnDomainRepository) MainToAddress(ctx context.Context, fellBack time.Time) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	if _, err := queries.ClearMainHost(ctx); err != nil {
		return "", ownDomainFailed("clearing the main address", err)
	}
	if !fellBack.IsZero() {
		if err := queries.MarkOwnDomainFellBack(ctx, timestamp(fellBack)); err != nil {
			return "", ownDomainFailed("keeping the fall-back", err)
		}
	}
	host, err := queries.MakeAddressMain(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", ownDomainFailed("making the address the main address again", err)
	}
	return host, nil
}

func (OwnDomainRepository) BrowserHost(ctx context.Context, host string) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	main, err := queries.BrowserHost(ctx, host)
	if err != nil {
		return "", ownDomainFailed("reading where a browser is sent", err)
	}
	return main, nil
}

func (OwnDomainRepository) IsMainHost(ctx context.Context, host string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	main, err := queries.IsMainHost(ctx, host)
	if err != nil {
		return false, ownDomainFailed("reading whether a host is a main address", err)
	}
	return main, nil
}
