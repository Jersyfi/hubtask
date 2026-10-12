// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The link that confirms a managed account's new address (ADR-0074 §6), on its way to that
// mailbox. Not a notification record, SendPasswordReset's reasoning: a credential with no
// preference to consult and nothing to list. The token is minted here, at delivery.

const (
	subjectAddressConfirm = "email.address_confirm.subject"
	bodyAddressConfirm    = "email.address_confirm.body"
)

// AddressMinter is the identity service's seam: one method and one value, so this package points
// at nothing the identity service owns.
type AddressMinter interface {
	MintAddressLink(ctx context.Context, tenantID, requestID shared.ID) (AddressLink, error)
}

// AddressLink is what the minter answers; an empty address is "nothing to send".
type AddressLink struct {
	Token       secret.Secret
	Address     string
	Locale      string
	DisplayName string
}

// SendAddressConfirmation is the job handler.
type SendAddressConfirmation struct {
	Links    AddressMinter
	Mail     mail.Sender
	Renderer i18n.Renderer
	// Workspaces and FallbackLocale are the recipient's language chain, SendPasswordReset's.
	Workspaces     WorkspaceReader
	UnitOfWork     persistence.UnitOfWork
	FallbackLocale string
	BaseURL        string
}

// Execute mints the link and sends it. A request replaced, confirmed or run out is finished
// business: the job completes.
func (s SendAddressConfirmation) Execute(ctx context.Context, tenantID, requestID shared.ID) error {
	link, err := s.Links.MintAddressLink(ctx, tenantID, requestID)
	if err != nil {
		return err
	}
	if link.Address == "" {
		return nil
	}
	reset := SendPasswordReset{
		Workspaces: s.Workspaces, UnitOfWork: s.UnitOfWork, FallbackLocale: s.FallbackLocale,
	}
	locale, err := reset.localeFor(ctx, tenantID, link.Locale)
	if err != nil {
		return err
	}
	params := map[string]string{
		"name": link.DisplayName,
		// In the fragment, the reset link's reasoning: nothing between the mail client and the
		// interface sees what follows the hash.
		"link": strings.TrimSuffix(s.BaseURL, "/") + "/confirm-address#token=" + url.PathEscape(link.Token.Reveal()),
	}
	return s.Mail.Send(ctx, mail.Message{
		To:      link.Address,
		Subject: s.Renderer.Render(locale, subjectAddressConfirm, params),
		Body:    s.Renderer.Render(locale, bodyAddressConfirm, params),
	})
}
