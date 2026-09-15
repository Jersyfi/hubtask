// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The two catalogue use cases of the sync context (N-03). A device is the person's, like a
// session: listing and forgetting are things a person asks for about their own account, which is
// what makes them catalogue entries where the pull and the push are not.
const (
	ListSyncDevicesName  = "ListSyncDevices"
	ForgetSyncDeviceName = "ForgetSyncDevice"

	// The scopes are the session listing's: reading one's own devices is reading one's own
	// account, and ending one is writing to it.
	devicesReadScope  = "accounts:read"
	devicesWriteScope = "accounts:write"

	deviceTarget = "sync_device"
)

// The audit codes. Forgetting a device ends a way into the workspace's data, the class of event
// a review looks for; the read declares one all the same, because a refused read is recorded
// against the action that was refused (audit.md §4).
const (
	DeviceForgottenAction audit.Action = "sync.device_forgotten"
	DeviceReadAction      audit.Action = "sync.device_read"
)

// SessionRevoker is the slice of the session repository forgetting a device needs: end the one
// session the device last synchronised under. Narrow rather than the whole port, because what
// the sync context may do to a sign-in is exactly this and nothing else.
type SessionRevoker interface {
	Revoke(ctx context.Context, sessionID, accountID shared.ID, at time.Time) (bool, error)
}

// DeviceWriter holds what both use cases share.
type DeviceWriter struct {
	Devices    repository.Devices
	Sessions   SessionRevoker
	Cursors    Cursors
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ListSyncDevices answers the caller's own devices - and never anybody else's, whatever the
// role (offline-sync.md §6, the session listing's rule).
type ListSyncDevices struct{ Writer DeviceWriter }

// Execute reads them, most recently seen first.
func (h ListSyncDevices) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Device, error) {
	if err := actor.RequireScope(devicesReadScope); err != nil {
		return nil, err
	}
	if actor.AccountID.IsZero() {
		return nil, shared.ErrForbidden.WithDetail("access.token_owner_required")
	}

	w := h.Writer
	var devices []domain.Device
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Devices.ForAccount(ctx, actor.AccountID)
		devices = found
		return err
	})
	return devices, err
}

// Descriptor is the catalogue entry.
func (h ListSyncDevices) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListSyncDevicesName,
		Summary: "The caller's own synchronising devices, most recently seen first: what each " +
			"said it is, when it last synchronised, where it stands in the change log, and " +
			"whether it has been forgotten. Never anybody else's, whatever the role.",
		SideEffects: "None. Reads only.",
		TokenScope:  devicesReadScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: DeviceReadAction, TargetType: deviceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListSyncDevices) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	devices, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(devices))
	for _, device := range devices {
		rows = append(rows, h.Writer.deviceOutput(device))
	}
	return usecase.Output{"data": rows}, nil
}

// ForgetSyncDevice ends what one of the caller's devices holds (offline-sync.md §6): the session
// it last synchronised under is revoked, and every contact from the identifier is refused until
// the sweep removes the row.
type ForgetSyncDevice struct{ Writer DeviceWriter }

// Execute forgets. Somebody else's device is not found rather than forbidden - whether a device
// identifier exists is nobody's business but its holder's - and forgetting twice is somebody
// making sure, not an error.
func (h ForgetSyncDevice) Execute(
	ctx context.Context, actor appshared.ActorContext, deviceID shared.ID,
) error {
	if err := actor.RequireScope(devicesWriteScope); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("access.token_owner_required")
	}
	if deviceID.IsZero() {
		return shared.ErrValidation.
			WithDetail("sync.device_not_found").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_not_found"})
	}

	w := h.Writer
	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		device, forgotten, err := w.Devices.Forget(ctx, deviceID, actor.AccountID, now)
		if err != nil {
			return err
		}
		if !forgotten {
			// Nothing changed: already forgotten, or not the caller's. The first is idempotent
			// success, the second the indistinguishable not-found - and the row says which,
			// without saying it to the caller any louder than that.
			existing, err := w.Devices.Find(ctx, deviceID)
			if err != nil || existing.AccountID != actor.AccountID {
				return shared.ErrNotFound.WithDetail("sync.device_not_found")
			}
			return nil
		}

		// The credential the device held. A session is ended; a token's identifier matches no
		// session and ends nothing, which is right for a credential the device did not mint.
		revoked := false
		if !device.CredentialID.IsZero() && w.Sessions != nil {
			revoked, err = w.Sessions.Revoke(ctx, device.CredentialID, actor.AccountID, now)
			if err != nil {
				return err
			}
		}
		return w.recordForgetting(ctx, actor, device, revoked, now)
	})
}

func (w DeviceWriter) recordForgetting(
	ctx context.Context, actor appshared.ActorContext, device domain.Device, revoked bool, at time.Time,
) error {
	sessionEnded := "false"
	if revoked {
		sessionEnded = "true"
	}
	// The platform travels: it is a class of machine, not a person's words. The display name
	// does not - it is what somebody typed, and the trail is not where it belongs (rule 10).
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     DeviceForgottenAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: deviceTarget,
		TargetID:   device.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "platform", Classification: audit.Open, To: device.Platform},
			audit.Change{Field: "session_ended", Classification: audit.Open, To: sessionEnded},
		),
	})
}

// Descriptor is the catalogue entry.
func (h ForgetSyncDevice) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ForgetSyncDeviceName,
		Summary: "Forgets one of the caller's own synchronising devices: the session it last " +
			"synchronised under is revoked, and every pull or push from its identifier is " +
			"refused until the client mints a new one. Somebody else's device is not found " +
			"rather than forbidden; forgetting twice is not an error.",
		SideEffects: "Marks the device, revokes its session and writes an audit entry. A repeat writes nothing.",
		TokenScope:  devicesWriteScope,
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "device_id", Kind: usecase.KindID, Required: true,
				Description: "The device to forget, from the caller's own list.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: DeviceForgottenAction, TargetType: deviceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A device is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ForgetSyncDevice) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	deviceID, err := in.ID("device_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// deviceOutput is the contract's shape (api/openapi.yaml, SyncDevice). The position travels as
// a cursor the device could resume from, minted at the moment it was last seen - so that a
// device restoring from the list stands where it stood, with the age it has.
func (w DeviceWriter) deviceOutput(device domain.Device) usecase.Output {
	out := usecase.Output{
		"id":           device.ID.String(),
		"platform":     nil,
		"display_name": nil,
		"last_seen_at": nil,
		"last_cursor":  nil,
		"blocked":      device.Blocked,
		"created_at":   device.CreatedAt,
	}
	if device.Platform != "" {
		out["platform"] = device.Platform
	}
	if device.DisplayName != "" {
		out["display_name"] = device.DisplayName
	}
	if !device.LastSeenAt.IsZero() {
		out["last_seen_at"] = device.LastSeenAt
	}
	if device.LastSeq > 0 && w.Cursors != nil {
		out["last_cursor"] = w.Cursors.Encode(Position{Seq: device.LastSeq, IssuedAt: device.LastSeenAt})
	}
	return out
}
