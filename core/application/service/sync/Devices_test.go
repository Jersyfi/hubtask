// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	session     = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	otherPerson = shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	deviceB     = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
)

// deviceStore is the device table in memory, with the adapter's refusals.
type deviceStore struct {
	devices map[shared.ID]domain.Device
	touched []domain.Contact
}

func newDeviceStore() *deviceStore { return &deviceStore{devices: map[shared.ID]domain.Device{}} }

func (s *deviceStore) Touch(_ context.Context, contact domain.Contact) (domain.Device, error) {
	s.touched = append(s.touched, contact)
	existing, known := s.devices[contact.DeviceID]
	switch {
	case known && existing.Blocked:
		return domain.Device{}, shared.ErrForbidden.WithDetail("sync.device_revoked")
	case known && existing.AccountID != contact.AccountID:
		return domain.Device{}, shared.ErrForbidden.WithDetail("sync.device_foreign")
	case !known:
		existing = domain.Device{ID: contact.DeviceID, TenantID: tenant, AccountID: contact.AccountID, CreatedAt: contact.Now}
	}
	if contact.Platform != "" {
		existing.Platform = contact.Platform
	}
	if contact.DisplayName != "" {
		existing.DisplayName = contact.DisplayName
	}
	if contact.LastSeq > existing.LastSeq {
		existing.LastSeq = contact.LastSeq
	}
	existing.LastSeenAt = contact.Now
	if !contact.CredentialID.IsZero() {
		existing.CredentialID = contact.CredentialID
	}
	s.devices[contact.DeviceID] = existing
	return existing, nil
}

func (s *deviceStore) ForAccount(_ context.Context, accountID shared.ID) ([]domain.Device, error) {
	var out []domain.Device
	for _, device := range s.devices {
		if device.AccountID == accountID {
			out = append(out, device)
		}
	}
	return out, nil
}

func (s *deviceStore) Forget(_ context.Context, id, accountID shared.ID, now time.Time) (domain.Device, bool, error) {
	device, known := s.devices[id]
	if !known || device.AccountID != accountID || device.Blocked {
		return domain.Device{}, false, nil
	}
	before := device
	device.Blocked, device.LastSeenAt = true, now
	s.devices[id] = device
	return before, true, nil
}

func (s *deviceStore) Find(_ context.Context, id shared.ID) (domain.Device, error) {
	device, known := s.devices[id]
	if !known {
		return domain.Device{}, shared.ErrNotFound
	}
	return device, nil
}

type sessionRevoker struct{ revoked []shared.ID }

func (r *sessionRevoker) Revoke(_ context.Context, sessionID, _ shared.ID, _ time.Time) (bool, error) {
	r.revoked = append(r.revoked, sessionID)
	return true, nil
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type deviceFixture struct {
	writer   DeviceWriter
	devices  *deviceStore
	sessions *sessionRevoker
	audit    *auditSink
}

func devicesOf(t *testing.T) deviceFixture {
	t.Helper()
	devices, sessions, sink := newDeviceStore(), &sessionRevoker{}, &auditSink{}
	return deviceFixture{
		writer: DeviceWriter{
			Devices: devices, Sessions: sessions, Cursors: cursors{}, Audit: sink,
			UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
		},
		devices: devices, sessions: sessions, audit: sink,
	}
}

func signedIn() appshared.ActorContext {
	a := actor()
	a.TokenID = session
	a.Scopes = []string{"items:read", "accounts:read", "accounts:write"}
	return a
}

func TestAPullRegistersTheDeviceWithTheRequestsPositionAndCredential(t *testing.T) {
	pull, f := pulling(t, entry(1, readable))
	devices := newDeviceStore()
	pull.Devices = devices

	_, err := pull.Pull(t.Context(), signedIn(), PullRequest{
		DeviceID: device, Platform: "hubctl", DisplayName: "Anna's laptop", Cursor: cursorAt(f, 0), Limit: 10,
	})
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	registered := devices.devices[device]
	if registered.AccountID != account || registered.Platform != "hubctl" ||
		registered.DisplayName != "Anna's laptop" || registered.CredentialID != session ||
		!registered.LastSeenAt.Equal(now) {
		t.Errorf("registered as %+v", registered)
	}

	// The next pull stands at the cursor it sends, and that is what the row remembers.
	_, err = pull.Pull(t.Context(), signedIn(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 1), Limit: 10})
	if err != nil {
		t.Fatalf("pulling again: %v", err)
	}
	if devices.devices[device].LastSeq != 1 {
		t.Errorf("the position is %d, want the cursor's 1", devices.devices[device].LastSeq)
	}
}

func TestADeviceTheAccountMayNotUseGetsNoPage(t *testing.T) {
	pull, f := pulling(t, entry(1, readable))
	devices := newDeviceStore()
	devices.devices[device] = domain.Device{ID: device, AccountID: otherPerson}
	devices.devices[deviceB] = domain.Device{ID: deviceB, AccountID: account, Blocked: true}
	pull.Devices = devices

	for id, code := range map[shared.ID]string{device: "sync.device_foreign", deviceB: "sync.device_revoked"} {
		_, err := pull.Pull(t.Context(), signedIn(), PullRequest{DeviceID: id, Cursor: cursorAt(f, 0)})
		if !errors.Is(err, shared.ErrForbidden) || shared.AsError(err).DetailCode != code {
			t.Errorf("%s answered %v, want %s", id, err, code)
		}
	}
	// A device identifier the client did not mint as a UUIDv7 is refused before it is looked up.
	_, err := pull.Pull(t.Context(), signedIn(), PullRequest{
		DeviceID: shared.MustParseID("0192f000-0000-4000-8000-0000000000d1"), Cursor: cursorAt(f, 0),
	})
	if got := shared.AsError(err).DetailCode; got != "sync.id_not_uuidv7" {
		t.Errorf("a v4 identifier was refused with %q", got)
	}
	if len(devices.touched) != 2 {
		t.Errorf("%d contacts reached the store, want the two with a well-formed identifier", len(devices.touched))
	}
}

func TestListingAnswersOnlyTheCallersDevicesInTheContractsShape(t *testing.T) {
	f := devicesOf(t)
	f.devices.devices[device] = domain.Device{
		ID: device, AccountID: account, Platform: "ios", DisplayName: "Anna's phone",
		LastSeq: 42, LastSeenAt: now, CreatedAt: now.Add(-time.Hour),
	}
	f.devices.devices[deviceB] = domain.Device{ID: deviceB, AccountID: otherPerson}

	out, err := ListSyncDevices{Writer: f.writer}.Descriptor().Handler.Invoke(
		t.Context(), signedIn(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("%d rows, want the caller's one", len(rows))
	}
	row := rows[0]
	wantCursor := (cursors{}).Encode(Position{Seq: 42, IssuedAt: now})
	if row["id"] != device.String() || row["platform"] != "ios" || row["display_name"] != "Anna's phone" ||
		row["blocked"] != false || row["last_cursor"] != wantCursor {
		t.Errorf("the row is %v", row)
	}

	limited := signedIn()
	limited.Scopes = []string{"items:read"}
	if _, err := (ListSyncDevices{Writer: f.writer}).Execute(t.Context(), limited); err == nil {
		t.Errorf("a token without accounts:read listed devices")
	}
}

func TestForgettingRevokesTheSessionAndWritesTheTrail(t *testing.T) {
	f := devicesOf(t)
	f.devices.devices[device] = domain.Device{ID: device, AccountID: account, Platform: "ios", DisplayName: "Anna's phone", CredentialID: session}

	if err := (ForgetSyncDevice{Writer: f.writer}).Execute(t.Context(), signedIn(), device); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
	if !f.devices.devices[device].Blocked {
		t.Errorf("the device is not blocked")
	}
	if len(f.sessions.revoked) != 1 || f.sessions.revoked[0] != session {
		t.Errorf("revoked %v, want the device's session", f.sessions.revoked)
	}
	if len(f.audit.entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(f.audit.entries))
	}
	entry := f.audit.entries[0]
	if entry.Action != DeviceForgottenAction || entry.TargetID != device || entry.Severity != audit.SeverityNotice {
		t.Errorf("the entry is %+v", entry)
	}
	if _, named := entry.Changes["display_name"]; named {
		t.Errorf("the display name reached the trail (rule 10): %v", entry.Changes)
	}
	if entry.Changes["session_ended"] == nil || entry.Changes["platform"] == nil {
		t.Errorf("the trail lacks the platform or the session outcome: %v", entry.Changes)
	}

	// Again: nothing changes, nothing is written, no error.
	if err := (ForgetSyncDevice{Writer: f.writer}).Execute(t.Context(), signedIn(), device); err != nil {
		t.Errorf("forgetting twice: %v", err)
	}
	if len(f.audit.entries) != 1 || len(f.sessions.revoked) != 1 {
		t.Errorf("a repeat wrote something")
	}
}

func TestSomebodyElsesDeviceIsNotFound(t *testing.T) {
	f := devicesOf(t)
	f.devices.devices[deviceB] = domain.Device{ID: deviceB, AccountID: otherPerson}

	err := (ForgetSyncDevice{Writer: f.writer}).Execute(t.Context(), signedIn(), deviceB)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("forgetting somebody else's device answered %v", err)
	}
	if f.devices.devices[deviceB].Blocked {
		t.Errorf("somebody else's device was blocked")
	}
}
