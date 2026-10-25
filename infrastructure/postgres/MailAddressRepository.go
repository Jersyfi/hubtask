// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MailAddressRepository is the places' addresses on the receiving domain (ADR-0092).
type MailAddressRepository struct{}

// NewMailAddressRepository builds the repository.
func NewMailAddressRepository() MailAddressRepository { return MailAddressRepository{} }

var _ repository.MailAddresses = MailAddressRepository{}

func (MailAddressRepository) TenantOf(ctx context.Context, code string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	tenant, err := queries.ResolveMailAddress(ctx, code)
	if err != nil {
		return "", shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("resolving a mail address: %w", err))
	}
	if !tenant.Valid {
		return "", mailAddressGone()
	}
	return idFrom(tenant)
}

func (MailAddressRepository) Live(
	ctx context.Context, target domain.MailTarget,
) (domain.MailAddress, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.MailAddress{}, err
	}
	id, err := optionalUUID(target.ID)
	if err != nil {
		return domain.MailAddress{}, err
	}
	row, err := queries.FindLiveMailAddress(ctx, sqlc.FindLiveMailAddressParams{
		TargetKind: string(target.Kind), TargetID: id,
	})
	if err != nil {
		if IsNoRows(err) {
			return domain.MailAddress{}, mailAddressGone()
		}
		return domain.MailAddress{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a mail address: %w", err))
	}
	return mailAddressFrom(row)
}

func (MailAddressRepository) ByCode(ctx context.Context, code string) (domain.MailAddress, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.MailAddress{}, err
	}
	row, err := queries.FindMailAddressByCode(ctx, code)
	if err != nil {
		if IsNoRows(err) {
			return domain.MailAddress{}, mailAddressGone()
		}
		return domain.MailAddress{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a mail address: %w", err))
	}
	return mailAddressFrom(sqlc.FindLiveMailAddressRow(row))
}

func (MailAddressRepository) Replace(
	ctx context.Context, address domain.MailAddress, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	target, err := optionalUUID(address.Target.ID)
	if err != nil {
		return err
	}
	if err := queries.RevokeMailAddress(ctx, sqlc.RevokeMailAddressParams{
		RevokedAt: timestampOf(at), TargetKind: string(address.Target.Kind), TargetID: target,
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking a mail address: %w", err))
	}
	id, err := uuidOf(address.ID)
	if err != nil {
		return err
	}
	openedBy, err := optionalUUID(address.OpenedBy)
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(address.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.InsertMailAddress(ctx, sqlc.InsertMailAddressParams{
		ID: id, TargetKind: string(address.Target.Kind), TargetID: target, Code: address.Code,
		SenderPolicy: string(address.Policy), OpenedBy: openedBy, CreatedBy: createdBy,
		CreatedAt: timestampOf(address.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			// Two first reads racing to mint the same place's address: the loser's is the conflict,
			// and its caller reads the winner's.
			return shared.ErrConflict.WithDetail("mail.address_raced")
		}
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing a mail address: %w", err))
	}
	return nil
}

func (MailAddressRepository) SetPolicy(ctx context.Context, address domain.MailAddress) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(address.ID)
	if err != nil {
		return err
	}
	openedBy, err := optionalUUID(address.OpenedBy)
	if err != nil {
		return err
	}
	changed, err := queries.SetMailAddressPolicy(ctx, sqlc.SetMailAddressPolicyParams{
		SenderPolicy: string(address.Policy), OpenedBy: openedBy, ID: id,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("setting a mail address's policy: %w", err))
	}
	if changed == 0 {
		return mailAddressGone()
	}
	return nil
}

func (MailAddressRepository) CountDelivery(
	ctx context.Context, id shared.ID, now time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountMailDelivery(ctx, sqlc.CountMailDeliveryParams{
		Now: timestampOf(now), ID: key,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting a mail delivery: %w", err))
	}
	return int(count), nil
}

// mailAddressFrom reads a row back, defensively: the row outlives the release that wrote it.
func mailAddressFrom(row sqlc.FindLiveMailAddressRow) (domain.MailAddress, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.MailAddress{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return domain.MailAddress{}, err
	}
	targetID, err := optionalID(row.TargetID)
	if err != nil {
		return domain.MailAddress{}, err
	}
	address := domain.MailAddress{
		ID: id, TenantID: tenant,
		Target: domain.MailTarget{Kind: domain.TargetKind(row.TargetKind), ID: targetID},
		Code:   row.Code, Policy: domain.SenderPolicy(row.SenderPolicy),
		CreatedAt: timeFrom(row.CreatedAt),
	}
	if address.OpenedBy, err = optionalID(row.OpenedBy); err != nil {
		return domain.MailAddress{}, err
	}
	if address.CreatedBy, err = optionalID(row.CreatedBy); err != nil {
		return domain.MailAddress{}, err
	}
	return address, nil
}

// mailAddressGone is the one answer for every address that leads nowhere (T-21).
func mailAddressGone() error {
	return shared.ErrNotFound.WithDetail("mail.address_not_found")
}
