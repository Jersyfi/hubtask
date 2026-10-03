// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The installation's half of ADR-0076: a withdrawal is announced for a date, *Withdraw now* repeats
// the count, either can be cancelled, and the form no longer ends an offer by itself.

const withdrawnRow = shared.ID("01936f2a-7c1e-7000-8000-0000000000d4")

// offeredRow puts an installation's provider into the store, switched on in twelve workspaces.
func offeredRow(store *instanceProviderStore) {
	store.rows = append(store.rows, domain.IdentityProvider{
		ID: withdrawnRow, Issuer: "https://login.platform.example", ClientID: "hubtask",
		DisplayName: "The platform", Kind: domain.KindGeneric,
		Provisioning: domain.ProvisionInvitedOnly, Enabled: true, Version: 1,
		OfferedWorkspaces: 12,
	})
}

func detailOf(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}

func TestAWithdrawalIsAnnouncedTwoWeeksAheadUnlessADateIsNamed(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	journal, _ := writer.Instance.Journal.(*journalStore)

	announced, err := WithdrawInstanceIdentityProvider{Writer: writer}.Execute(
		t.Context(), operator(), WithdrawCommand{ID: withdrawnRow})
	if err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if want := fixed.Add(domain.WithdrawalNotice); !announced.WithdrawAt.Equal(want) {
		t.Errorf("announced for %v, want %v", announced.WithdrawAt, want)
	}
	// Until the date it keeps working everywhere.
	if !announced.OfferedAt(fixed) {
		t.Error("an announced withdrawal ended the offer at once")
	}

	named := fixed.Add(72 * time.Hour)
	moved, err := WithdrawInstanceIdentityProvider{Writer: writer}.Execute(
		t.Context(), operator(), WithdrawCommand{ID: withdrawnRow, At: named})
	if err != nil {
		t.Fatalf("announcing for a named date: %v", err)
	}
	if !moved.WithdrawAt.Equal(named) {
		t.Errorf("announced for %v, want the named %v", moved.WithdrawAt, named)
	}

	// The installation's own evidence: the act, the date and the number - never a workspace.
	if journal == nil || len(journal.entries) != 2 {
		t.Fatalf("the journal holds %v", journal)
	}
	entry := journal.entries[1]
	if entry.Action != journalProviderWithdrawn || entry.Details["offered_workspaces"] != 12 ||
		entry.Details["withdraw_at"] != named.UTC().Format(time.RFC3339) {
		t.Errorf("the journal entry reads %+v", entry)
	}
}

func TestWithdrawNowRepeatsTheCount(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	withdraw := WithdrawInstanceIdentityProvider{Writer: writer}

	for _, count := range []*int{nil, ptr(11)} {
		_, err := withdraw.Execute(t.Context(), operator(),
			WithdrawCommand{ID: withdrawnRow, At: fixed, ConfirmCount: count})
		if !errors.Is(err, shared.ErrValidation) ||
			detailOf(err) != "identity_provider.withdraw_count_mismatch" {
			t.Errorf("withdrawing now with %v answered %v", count, err)
		}
	}
	if !store.rows[0].WithdrawAt.IsZero() {
		t.Fatal("an unconfirmed withdraw-now was written")
	}

	// A date already past is now as well: a compromised provider is not offered for a moment longer.
	gone, err := withdraw.Execute(t.Context(), operator(),
		WithdrawCommand{ID: withdrawnRow, At: fixed.Add(-time.Hour), ConfirmCount: ptr(12)})
	if err != nil {
		t.Fatalf("withdrawing now with the count: %v", err)
	}
	if gone.OfferedAt(fixed) {
		t.Error("withdrawn now and still offered")
	}
}

func TestCancellingAWithdrawalKeepsOfferingIt(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	store.rows[0].WithdrawAt = fixed.Add(-time.Hour) // already ended

	kept, err := CancelInstanceIdentityProviderWithdrawal{Writer: writer}.Execute(
		t.Context(), operator(), withdrawnRow, "")
	if err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	// The identities were never removed, so offering it again is all that sign-in needs.
	if !kept.WithdrawAt.IsZero() || !kept.OfferedAt(fixed) {
		t.Errorf("after cancelling: withdraw_at %v, offered %v", kept.WithdrawAt, kept.OfferedAt(fixed))
	}
	if kept.OfferedWorkspaces != 12 {
		t.Errorf("the count moved to %d", kept.OfferedWorkspaces)
	}
}

// The form no longer ends an offer by itself: an unannounced, unconfirmed withdrawal is exactly what
// ADR-0076 replaces. The same value, or none, is accepted.
func TestTheInstallationsFormRefusesAChangedSwitch(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	configure := ConfigureInstanceIdentityProvider{Writer: writer}
	cmd := identityservice.ConfigureIdentityProviderCommand{
		ID: withdrawnRow, Issuer: "https://login.platform.example", ClientID: "hubtask",
		ClientSecret: secret.New("s3cr3t"),
	}

	off := false
	cmd.Enabled = &off
	if _, err := configure.Execute(t.Context(), operator(), cmd); detailOf(err) != "identity_provider.withdraw_instead" {
		t.Errorf("switching off on the form answered %v", err)
	}
	on := true
	for _, enabled := range []*bool{&on, nil} {
		cmd.Enabled = enabled
		saved, err := configure.Execute(t.Context(), operator(), cmd)
		if err != nil {
			t.Fatalf("saving with enabled %v: %v", enabled, err)
		}
		if !saved.Enabled {
			t.Error("the save switched the offer off")
		}
	}
}

// The count is the operator's: the installation's listing answers it, a workspace's never does.
func TestTheCountIsAnsweredToTheOperatorAndNamesNoWorkspace(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	registry, err := usecase.NewRegistry(nil, ListInstanceIdentityProviders{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	listed, err := registry.Invoke(t.Context(), ListInstanceIdentityProvidersName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["offered_workspaces"] != 12 {
		t.Fatalf("the listing answered %v", listed)
	}
	if _, held := identityservice.ProviderOutput(store.rows[0])["offered_workspaces"]; held {
		t.Error("the projection a workspace reads carries the count")
	}
}

func TestTheWithdrawalUseCasesGoThroughTheRegistry(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	registry, err := usecase.NewRegistry(nil,
		WithdrawInstanceIdentityProvider{Writer: writer}.Descriptor(),
		CancelInstanceIdentityProviderWithdrawal{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	answered, err := registry.Invoke(t.Context(), WithdrawInstanceIdentityProviderName, operator(),
		usecase.Input{"id": string(withdrawnRow), "withdraw_at": fixed.Add(-time.Minute).Format(time.RFC3339),
			"confirm_count": 12})
	if err != nil {
		t.Fatalf("withdrawing now through the registry: %v", err)
	}
	if answered["withdraw_at"] == nil || answered["offered_workspaces"] != 12 {
		t.Errorf("the answer reads %v", answered)
	}
	if _, err := registry.Invoke(t.Context(), CancelInstanceIdentityProviderWithdrawalName, operator(),
		usecase.Input{"id": string(withdrawnRow)}); err != nil {
		t.Fatalf("cancelling through the registry: %v", err)
	}
	if !store.rows[0].WithdrawAt.IsZero() {
		t.Error("the cancellation left the date")
	}

	// The pair every operation at this level demands.
	outsider, _, _, _ := newInstanceProviderWriter(newRegister(secondOperator))
	if _, err := (WithdrawInstanceIdentityProvider{Writer: outsider}).Execute(
		t.Context(), operator(), WithdrawCommand{ID: withdrawnRow}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a writer outside the register answered %v", err)
	}
}

func ptr(n int) *int { return &n }

// A withdrawal is no way back: an offer that has already ended is refused rather than given a
// fresh notice, which is what would revive a provider withdrawn now because it was compromised.
// Only the cancellation offers it again.
func TestWithdrawingAnEndedOfferDoesNotReviveIt(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	store.rows[0].WithdrawAt = fixed.Add(-time.Hour) // withdrawn now, an hour ago

	_, err := WithdrawInstanceIdentityProvider{Writer: writer}.Execute(
		t.Context(), operator(), WithdrawCommand{ID: withdrawnRow})
	if detailOf(err) != "identity_provider.already_withdrawn" {
		t.Fatalf("withdrawing an ended offer answered %v", err)
	}
	if !store.rows[0].WithdrawAt.Equal(fixed.Add(-time.Hour)) {
		t.Errorf("the ended offer was moved to %v", store.rows[0].WithdrawAt)
	}
}

// Less than a day ahead is no notice: it asks for the count as Withdraw now does, so the
// confirmation cannot be skipped by naming a moment a second away.
func TestADateLessThanADayAheadAsksForTheCount(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	offeredRow(store)
	withdraw := WithdrawInstanceIdentityProvider{Writer: writer}
	soon := fixed.Add(time.Minute)

	if _, err := withdraw.Execute(t.Context(), operator(), WithdrawCommand{ID: withdrawnRow, At: soon}); detailOf(err) !=
		"identity_provider.withdraw_count_mismatch" {
		t.Fatalf("a minute's notice without the count answered %v", err)
	}
	announced, err := withdraw.Execute(t.Context(), operator(),
		WithdrawCommand{ID: withdrawnRow, At: soon, ConfirmCount: ptr(12)})
	if err != nil {
		t.Fatalf("a minute's notice with the count: %v", err)
	}
	if !announced.WithdrawAt.Equal(soon) {
		t.Errorf("the confirmed short notice ends at %v, want %v", announced.WithdrawAt, soon)
	}
	if _, err := withdraw.Execute(t.Context(), operator(),
		WithdrawCommand{ID: withdrawnRow, At: fixed.Add(domain.MinimumWithdrawalNotice)}); err != nil {
		t.Errorf("a day's notice asked for the count: %v", err)
	}
}
