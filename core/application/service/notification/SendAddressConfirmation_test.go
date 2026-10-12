// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type addressLinks struct{ link AddressLink }

func (a addressLinks) MintAddressLink(context.Context, shared.ID, shared.ID) (AddressLink, error) {
	return a.link, nil
}

// The confirmation goes to the address it confirms, in the account's language, its token in the
// link's fragment.
func TestTheAddressConfirmationCarriesItsLinkInTheFragment(t *testing.T) {
	mailer := &mailbox{}
	sender := SendAddressConfirmation{
		Links: addressLinks{AddressLink{
			Token: secret.New("hbt_mfa_token"), Address: "lena@example.org", Locale: "de", DisplayName: "Lena",
		}},
		Mail: mailer, Renderer: catalogue{}, FallbackLocale: "en", BaseURL: "https://family.example/",
	}
	if err := sender.Execute(t.Context(), tenant, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a7")); err != nil {
		t.Fatalf("sending: %v", err)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("%d messages sent", len(mailer.sent))
	}
	message := mailer.sent[0]
	if message.To != "lena@example.org" || !strings.HasPrefix(message.Subject, "[de] email.address_confirm.subject") {
		t.Errorf("the message reads %+v", message)
	}
	if !strings.Contains(message.Body, "link=https://family.example/confirm-address#token=hbt_mfa_token") ||
		!strings.Contains(message.Body, "name=Lena") {
		t.Errorf("the body reads %q", message.Body)
	}
}

// A request replaced, confirmed or run out before its mail sends nothing, and the job completes.
func TestAnAddressLinkNobodyCanUseIsNotSent(t *testing.T) {
	mailer := &mailbox{}
	sender := SendAddressConfirmation{Links: addressLinks{}, Mail: mailer, Renderer: catalogue{}}
	if err := sender.Execute(t.Context(), tenant, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a7")); err != nil {
		t.Fatalf("sending: %v", err)
	}
	if len(mailer.sent) != 0 {
		t.Errorf("sent %v", mailer.sent)
	}
}
