// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	notificationrepo "github.com/Jersyfi/hubtask/core/application/repository/notification"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// preferenceStore is the notification_preference table, in memory: what was written and nothing
// about what was not.
type preferenceStore struct {
	rows  []notification.Preference
	saved []notification.Preference
}

func (s *preferenceStore) Find(
	_ context.Context, account shared.ID, category notification.Category, channel notification.Channel,
) (notification.Preference, error) {
	for _, row := range s.rows {
		if row.AccountID == account && row.Category == category && row.Channel == channel {
			return row, nil
		}
	}
	return notification.Preference{}, shared.ErrNotFound
}

func (s *preferenceStore) Save(_ context.Context, preference notification.Preference) error {
	s.saved = append(s.saved, preference)
	for i, row := range s.rows {
		if row.AccountID == preference.AccountID && row.Category == preference.Category && row.Channel == preference.Channel {
			s.rows[i] = preference
			return nil
		}
	}
	s.rows = append(s.rows, preference)
	return nil
}

func (s *preferenceStore) ListForAccount(_ context.Context, account shared.ID) ([]notification.Preference, error) {
	var rows []notification.Preference
	for _, row := range s.rows {
		if row.AccountID == account {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

var _ notificationrepo.Preferences = (*preferenceStore)(nil)

func preferenceReader() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{accountsRead}
	return actor
}

func adminAccount(t *testing.T) domain.Account {
	t.Helper()
	account, err := domain.Invite(adminID, tenant, "anna@example.org", "Anna")
	if err != nil {
		t.Fatalf("preparing the account: %v", err)
	}
	return account
}

func listPreferences(accounts *accountStore, prefs *preferenceStore, auth *authorizer) ListNotificationPreferences {
	return ListNotificationPreferences{
		Accounts: accounts, Preferences: prefs, Authorizer: auth, UnitOfWork: &unitOfWork{},
	}
}

// Every category and channel is in the answer, the stored row where there is one and the default
// marked as such where there is not.
func TestTheOwnPreferencesAreAnsweredWithDefaultsMarked(t *testing.T) {
	off := notification.DefaultPreference(tenant, adminID, notification.CategoryComment, notification.ChannelEmail)
	off.Enabled = false
	off.UpdatedAt = now
	prefs := &preferenceStore{rows: []notification.Preference{off}}
	auth := &authorizer{}

	effective, err := listPreferences(newAccounts(adminAccount(t)), prefs, auth).
		Execute(t.Context(), preferenceReader(), "")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	want := len(notification.Categories()) * len(notification.Channels())
	if len(effective) != want {
		t.Fatalf("%d rows, want one per category and channel (%d)", len(effective), want)
	}
	if len(auth.requests) != 0 {
		t.Error("reading one's own settings asked a permission question")
	}
	for _, row := range effective {
		switch {
		case row.Category == notification.CategoryComment && (row.IsDefault || row.Enabled):
			t.Errorf("the stored row came back as %+v", row)
		case row.Category != notification.CategoryComment && (!row.IsDefault || !row.Enabled || !row.IncludeTitle):
			t.Errorf("a pair nobody wrote came back as %+v, want the default marked", row)
		}
	}
}

// Somebody else's settings ask the member management permission, and a refusal is the refusal.
func TestAnotherAccountsPreferencesNeedTheMemberManagementPermission(t *testing.T) {
	auth := &authorizer{}
	if _, err := listPreferences(newAccounts(invitedAccount(t)), &preferenceStore{}, auth).
		Execute(t.Context(), preferenceReader(), invitedID); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(auth.requests) != 1 || auth.requests[0].TargetID != invitedID {
		t.Fatalf("the questions asked were %+v", auth.requests)
	}

	refused := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	_, err := listPreferences(newAccounts(invitedAccount(t)), &preferenceStore{}, refused).
		Execute(t.Context(), preferenceReader(), invitedID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("got %v, want the refusal", err)
	}
}

func TestPreferencesOfAnAccountThatIsNotHereAreNotFound(t *testing.T) {
	_, err := listPreferences(newAccounts(), &preferenceStore{}, &authorizer{}).
		Execute(t.Context(), preferenceReader(), invitedID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("got %v, want not found", err)
	}
}

func TestACredentialWithoutTheScopeReadsNoPreferences(t *testing.T) {
	_, err := listPreferences(newAccounts(adminAccount(t)), &preferenceStore{}, &authorizer{}).
		Execute(t.Context(), admin(), "")
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("got %v, want the scope refusal", err)
	}
}

func TestTheListDescriptorAcceptsTheControllersInput(t *testing.T) {
	descriptor := ListNotificationPreferences{}.Descriptor()
	if err := descriptor.ValidateInput(usecase.Input{"account_id": adminID.String()}); err != nil {
		t.Errorf("the controller's input is refused: %v", err)
	}
	out, err := listPreferences(newAccounts(adminAccount(t)), &preferenceStore{}, &authorizer{}).
		invoke(t.Context(), preferenceReader(), usecase.Input{})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) == 0 || rows[0]["updated_at"] != nil || rows[0]["is_default"] != true {
		t.Errorf("a default row renders as %v", rows)
	}
}

func preferenceWriter() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{accountsWrite}
	return actor
}

func setPreference(accounts *accountStore, prefs *preferenceStore, auth *authorizer, sink *auditSink) SetNotificationPreference {
	return SetNotificationPreference{
		Accounts: accounts, Preferences: prefs, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

// C-09's acceptance, now reachable from a client: a category switched off makes the next
// notification of that kind SUPPRESSED with the record saying why - proved by handing the row
// the write stored to the decision the delivery reads.
func TestSwitchingACategoryOffSuppressesTheNextNotification(t *testing.T) {
	prefs, sink := &preferenceStore{}, &auditSink{}

	written, err := setPreference(newAccounts(adminAccount(t)), prefs, &authorizer{}, sink).
		Execute(t.Context(), preferenceWriter(), SetNotificationPreferenceCommand{
			Category: notification.CategoryComment, Channel: notification.ChannelEmail,
			Enabled: false, IncludeTitle: true,
		})
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if written.IsDefault || written.Enabled || written.UpdatedAt != now {
		t.Errorf("written %+v", written)
	}

	stored, err := prefs.Find(t.Context(), adminID, notification.CategoryComment, notification.ChannelEmail)
	if err != nil {
		t.Fatalf("the row was not stored: %v", err)
	}
	decision := notification.Decide(
		notification.Notification{TenantID: tenant, RecipientID: adminID, Category: notification.CategoryComment},
		notification.Recipient{AccountID: adminID, HasAddress: true}, stored)
	if decision.Send || decision.Reason != notification.ReasonCategoryOff {
		t.Errorf("the next notification decided %+v, want suppressed for the category", decision)
	}

	if len(sink.entries) != 1 || sink.entries[0].Action != NotificationPreferenceChangedAction {
		t.Errorf("audit entries %+v, want the change recorded", sink.entries)
	}
}

// The title is switchable on its own: the category stays on and the email stops naming the entry.
func TestWithholdingTheTitleLeavesTheCategoryOn(t *testing.T) {
	prefs := &preferenceStore{}

	if _, err := setPreference(newAccounts(adminAccount(t)), prefs, &authorizer{}, &auditSink{}).
		Execute(t.Context(), preferenceWriter(), SetNotificationPreferenceCommand{
			Category: notification.CategoryAssignment, Channel: notification.ChannelEmail,
			Enabled: true, IncludeTitle: false,
		}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	stored, _ := prefs.Find(t.Context(), adminID, notification.CategoryAssignment, notification.ChannelEmail)
	decision := notification.Decide(
		notification.Notification{TenantID: tenant, RecipientID: adminID, Category: notification.CategoryAssignment},
		notification.Recipient{AccountID: adminID, HasAddress: true}, stored)
	if !decision.Send || decision.IncludeTitle {
		t.Errorf("decided %+v, want sent without the title", decision)
	}
}

func TestAnUnknownCategoryOrChannelIsRefusedByName(t *testing.T) {
	cases := []struct {
		name string
		cmd  SetNotificationPreferenceCommand
		code string
	}{
		{"category", SetNotificationPreferenceCommand{Category: "PIGEON", Channel: notification.ChannelEmail}, "notifications.category_unknown"},
		{"channel", SetNotificationPreferenceCommand{Category: notification.CategoryComment, Channel: "FAX"}, "notifications.channel_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefs := &preferenceStore{}
			_, err := setPreference(newAccounts(adminAccount(t)), prefs, &authorizer{}, &auditSink{}).
				Execute(t.Context(), preferenceWriter(), tc.cmd)
			if !errors.Is(err, shared.ErrValidation) || shared.AsError(err).DetailCode != tc.code {
				t.Errorf("got %v, want %s", err, tc.code)
			}
			if len(prefs.saved) != 0 {
				t.Error("a row was written for a pair that does not exist")
			}
		})
	}
}

// A viewer changes their own and cannot change another account's; an administrator can.
func TestAnotherAccountsPreferenceAsksTheMemberManagementPermission(t *testing.T) {
	refused := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	prefs := &preferenceStore{}
	_, err := setPreference(newAccounts(invitedAccount(t)), prefs, refused, &auditSink{}).
		Execute(t.Context(), preferenceWriter(), SetNotificationPreferenceCommand{
			AccountID: invitedID, Category: notification.CategoryComment, Channel: notification.ChannelEmail,
		})
	if !errors.Is(err, shared.ErrForbidden) || len(prefs.saved) != 0 {
		t.Errorf("got %v after %d writes, want the refusal and no write", err, len(prefs.saved))
	}

	allowed := &authorizer{}
	if _, err := setPreference(newAccounts(invitedAccount(t)), prefs, allowed, &auditSink{}).
		Execute(t.Context(), preferenceWriter(), SetNotificationPreferenceCommand{
			AccountID: invitedID, Category: notification.CategoryComment, Channel: notification.ChannelEmail,
		}); err != nil {
		t.Fatalf("an administrator was refused: %v", err)
	}
	if len(allowed.requests) != 1 || allowed.requests[0].TargetID != invitedID {
		t.Errorf("the questions asked were %+v", allowed.requests)
	}
}

func TestTheSetDescriptorAcceptsTheControllersInput(t *testing.T) {
	descriptor := SetNotificationPreference{}.Descriptor()
	in := usecase.Input{
		"account_id": adminID.String(), "category": "COMMENT", "channel": "EMAIL",
		"enabled": false, "include_title": true,
	}
	if err := descriptor.ValidateInput(in); err != nil {
		t.Errorf("the controller's input is refused: %v", err)
	}
	if !descriptor.Audit.Required {
		t.Error("a write of what a person is told about is audited")
	}
}
