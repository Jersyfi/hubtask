// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.SAMLCertificateWatches = IdentityProviderRepository{}

// SAMLWatches answers the workspace's SAML providers with what was last told about their end.
func (IdentityProviderRepository) SAMLWatches(ctx context.Context) ([]identity.SAMLWatch, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSamlCertificateWatches(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the SAML certificate watches: %w", err))
	}
	watches := make([]identity.SAMLWatch, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		watch := identity.SAMLWatch{ProviderID: id, DisplayName: row.DisplayName, Enabled: row.Enabled}
		if row.SamlCertificate != nil {
			watch.Certificate = *row.SamlCertificate
		}
		if row.SamlExpiryToldFor.Valid {
			watch.ToldFor = row.SamlExpiryToldFor.Time.UTC()
		}
		if row.SamlExpiryToldDays != nil {
			watch.ToldDays = int(*row.SamlExpiryToldDays)
		}
		if row.SamlCertificatesEnd.Valid {
			watch.RecordedEnd = row.SamlCertificatesEnd.Time.UTC()
		}
		watches = append(watches, watch)
	}
	return watches, nil
}

// MarkSAMLExpiryTold records the warning given for the end.
func (IdentityProviderRepository) MarkSAMLExpiryTold(ctx context.Context, id shared.ID, end time.Time, days int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if days < 0 || days > math.MaxInt16 {
		return shared.Internalf("postgres: a SAML warning of %d days", days)
	}
	told := int16(days)
	marked, err := queries.MarkSamlExpiryTold(ctx, sqlc.MarkSamlExpiryToldParams{
		ID: key, ToldFor: timestampOf(end), ToldDays: &told,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the SAML expiry warning: %w", err))
	}
	if marked == 0 {
		return shared.ErrNotFound.WithDetail("identity_provider.not_found")
	}
	return nil
}

// RecordSAMLCertificatesEnd keeps the certificates' end the watch found on the provider.
func (IdentityProviderRepository) RecordSAMLCertificatesEnd(ctx context.Context, id shared.ID, end time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	recorded, err := queries.RecordSamlCertificatesEnd(ctx, sqlc.RecordSamlCertificatesEndParams{
		ID: key, CertificatesEnd: timestampOf(end),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the SAML certificates' end: %w", err))
	}
	if recorded == 0 {
		return shared.ErrNotFound.WithDetail("identity_provider.not_found")
	}
	return nil
}

// SAMLCertificatesEnding counts the installation's SAML providers by their certificates' end, across
// every workspace, through the door that names none of them (migration 0355).
func (IdentityProviderRepository) SAMLCertificatesEnding(ctx context.Context) (identity.SAMLCertificateCount, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.SAMLCertificateCount{}, err
	}
	row, err := queries.SamlCertificatesEnding(ctx)
	if err != nil {
		return identity.SAMLCertificateCount{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the SAML certificates' ends: %w", err))
	}
	return identity.SAMLCertificateCount{
		Providers: row.Providers, Within30Days: row.Within30Days, Within7Days: row.Within7Days, Ended: row.Ended,
	}, nil
}
