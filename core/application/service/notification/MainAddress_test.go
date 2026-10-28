// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// mainBase is the workspace's address after its main row changed: no longer the subdomain its
// short name makes under baseURL, the installation's own address.
const (
	mainBase   = "https://tasks.anna.example"
	formerBase = "https://anna.hub.example.org"
)

// movedAddress is a workspace "anna" of a multi-mode installation at baseURL whose main row moved
// from its subdomain to tasks.anna.example. Every mail in this package's tests is written through
// it, so each kind's own test proves its link names the main row (UC-INS-19 check 9).
func movedAddress() identityservice.WorkspaceAddress {
	return identityservice.WorkspaceAddress{
		Workspaces: workspaceSlugged{slug: "anna"},
		Hosts: hostRows{
			{Host: "anna.hub.example.org", State: identity.HostActive},
			{Host: "tasks.anna.example", State: identity.HostActive, Canonical: true},
		},
		BaseURL: baseURL, Multi: true,
	}
}

type hostRows []identity.TenantHost

func (h hostRows) Insert(context.Context, identity.TenantHost) error { return nil }

func (h hostRows) List(context.Context) ([]identity.TenantHost, error) { return h, nil }

// fixedAddress is a workspace whose address is the one given, as single mode answers it.
type fixedAddress string

func (a fixedAddress) Of(context.Context) (string, error) { return string(a), nil }

// leadsToTheMainRow holds a message to its link on the main row, and to no other host.
func leadsToTheMainRow(t *testing.T, body, link string) {
	t.Helper()
	if !strings.Contains(body, "link="+mainBase+link) {
		t.Errorf("the body links elsewhere than %s%s: %q", mainBase, link, body)
	}
	if strings.Contains(body, formerBase) || strings.Contains(body, baseURL) {
		t.Errorf("the body names a host the workspace no longer answers at: %q", body)
	}
}

func TestTheResetLinkNamesTheMainRow(t *testing.T) {
	mailer := &mailbox{}
	err := SendPasswordReset{
		Resets: &resetMinter{link: ResetLink{
			Token: secret.New("hbt_mfa_0123_abc"), HasPassword: true, Address: "mara@contoso.example",
		}},
		Mail: mailer, Renderer: catalogue{}, FallbackLocale: "en", UnitOfWork: &unitOfWork{},
		Addresses: movedAddress(),
	}.Execute(t.Context(), tenant, anna)
	if err != nil || len(mailer.sent) != 1 {
		t.Fatalf("sending: %v, %d sent", err, len(mailer.sent))
	}
	leadsToTheMainRow(t, mailer.sent[0].Body, "/reset#token=hbt_mfa_0123_abc")
}

func TestTheAddressConfirmationNamesTheMainRow(t *testing.T) {
	mailer := &mailbox{}
	err := SendAddressConfirmation{
		Links: addressLinks{AddressLink{Token: secret.New("hbt_mfa_token"), Address: "lena@example.org"}},
		Mail:  mailer, Renderer: catalogue{}, FallbackLocale: "en", UnitOfWork: &unitOfWork{},
		Addresses: movedAddress(),
	}.Execute(t.Context(), tenant, shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000a7"))
	if err != nil || len(mailer.sent) != 1 {
		t.Fatalf("sending: %v, %d sent", err, len(mailer.sent))
	}
	leadsToTheMainRow(t, mailer.sent[0].Body, "/confirm-address#token=hbt_mfa_token")
}

func TestTheNoticeOfAChangedWayInNamesTheMainRow(t *testing.T) {
	mailer := &mailbox{}
	sender := wayInSender(mailer, providersHeld{})
	sender.Addresses = movedAddress()
	err := sender.Execute(t.Context(), tenant, WayInNotice{
		AccountID: anna, Change: identity.WayInChange{Event: identity.WayInPasskeyAdded, At: now},
	})
	if err != nil || len(mailer.sent) != 1 {
		t.Fatalf("sending: %v, %d sent", err, len(mailer.sent))
	}
	leadsToTheMainRow(t, mailer.sent[0].Body, "/profile/security")
}

func TestThePasswordOpeningNamesTheMainRow(t *testing.T) {
	mailer := &mailbox{}
	sender := openingSender(mailer)
	sender.Addresses = movedAddress()
	err := sender.Execute(t.Context(), tenant, OpeningNotice{
		AccountID: anna, Event: OpeningEventOpened, Until: time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC),
	})
	if err != nil || len(mailer.sent) != 1 {
		t.Fatalf("sending: %v, %d sent", err, len(mailer.sent))
	}
	leadsToTheMainRow(t, mailer.sent[0].Body, "/administration/sign-in")
}

// The notification mails - an entry, a rule, webhooks, a role's place, an emergency access, a
// trashed private hub - are written through movedAddress in their own tests; this one holds the
// item's link to nothing but the main row.
func TestANotificationNamesTheMainRow(t *testing.T) {
	fixture := delivery(t, domain.CategoryAssignment, true)
	if err := fixture.delivery.Execute(t.Context(), tenant, fixture.record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	leadsToTheMainRow(t, fixture.mailbox.sent[0].Body, "/items/"+itemID.String())
}
