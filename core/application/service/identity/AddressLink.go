// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// AddressLink is what the confirmation mail carries: the token, where it goes, and for whom.
type AddressLink struct {
	Token       secret.Secret
	Address     string
	Locale      string
	DisplayName string
}

// MintAddressLink mints the token of an address request when its mail goes out, the reset link's
// discipline: the queue carries the request's identifier only, so the plaintext exists once, in the
// message on its way.
type MintAddressLink struct {
	Requests   repository.AddressRequests
	Entropy    clock.Entropy
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// MintAddressLink answers the link, or an empty one where the request was replaced, confirmed or
// ran out before its mail - finished business, nothing a retry would find.
func (m MintAddressLink) MintAddressLink(ctx context.Context, tenantID, requestID shared.ID) (AddressLink, error) {
	material, err := m.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return AddressLink{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewPendingToken(tenantID, material)
	if err != nil {
		return AddressLink{}, err
	}
	var mail repository.AddressMail
	err = m.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		read, err := m.Requests.Mint(ctx, requestID, presented, m.Clock.Now())
		mail = read
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return AddressLink{}, nil
	}
	if err != nil {
		return AddressLink{}, err
	}
	return AddressLink{
		Token: secret.New(presented.Secret()), Address: mail.Address, Locale: mail.Locale,
		DisplayName: mail.DisplayName,
	}, nil
}
