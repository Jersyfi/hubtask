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
