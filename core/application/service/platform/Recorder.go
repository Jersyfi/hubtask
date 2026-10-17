// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package platform writes the installation's events: what a provider's purchase platform learns
// about a workspace (UC-INS-14, multi-tenancy.md §4.3). Every place that changes what such an
// event reports calls the one Recorder, inside the change's own transaction, so an event is written
// exactly when its change commits and carries the state the change left.
package platform

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Workspaces reads the transaction's own workspace, as the control plane does.
type Workspaces interface {
	Find(ctx context.Context) (adminrepo.TenantRecord, error)
}

// Recorder writes installation events. The zero value writes nothing, which is what a build or a
// test without the feed gets; single mode writes nothing either, since nothing there could read
// them (UC-INS-14 check 7).
type Recorder struct {
	Events     adminrepo.PlatformEvents
	Workspaces Workspaces
	IDs        clock.IDGenerator
	Tenancy    env.TenancyMode
}

// On reports whether events are written at all.
func (r Recorder) On() bool {
	return r.Events != nil && r.Tenancy == env.TenancyMulti
}

// Record writes one event about the transaction's workspace, with the state it reads there - the
// state the change in progress has left. `extra` holds the event's own fields.
func (r Recorder) Record(ctx context.Context, eventType event.Type, actor event.Actor, at time.Time,
	extra map[string]any,
) error {
	_, err := r.RecordOnce(ctx, eventType, actor, at, extra, "")
	return err
}

// RecordOnce is Record for a writer that may run twice: the event is written once per key, and the
// answer says whether this call wrote it.
func (r Recorder) RecordOnce(ctx context.Context, eventType event.Type, actor event.Actor, at time.Time,
	extra map[string]any, key string,
) (bool, error) {
	if !r.On() {
		return false, nil
	}
	record, err := r.Workspaces.Find(ctx)
	if err != nil {
		return false, err
	}
	return r.write(ctx, eventType, StateOf(record), actor, at, extra, key)
}

// OwnersChanged writes `owner_changed` about the transaction's workspace: who the owners are is the
// workspace's, and the event names none of them.
func (r Recorder) OwnersChanged(ctx context.Context, actor event.Actor, at time.Time) error {
	return r.Record(ctx, event.WorkspaceOwnerChanged, actor, at, nil)
}

// RecordDeleted writes the last event about a workspace its purge is removing, and removes the
// platform's reference from the workspace's kept events: the reference leaves with the workspace
// (multi-tenancy.md §4.3). The state is the last one read with the version one higher, so a
// receiver keeping the newest keeps this one.
func (r Recorder) RecordDeleted(ctx context.Context, last adminrepo.TenantRecord, actor event.Actor,
	at time.Time,
) error {
	if !r.On() {
		return nil
	}
	if err := r.Events.ForgetReference(ctx); err != nil {
		return err
	}
	state := StateOf(last)
	state.Status, state.ExternalReference, state.Version = event.WorkspaceDeletedStatus, "", state.Version+1
	_, err := r.write(ctx, event.WorkspaceDeleted, state, actor, at, nil, "")
	return err
}

func (r Recorder) write(ctx context.Context, eventType event.Type, state event.WorkspaceState,
	actor event.Actor, at time.Time, extra map[string]any, key string,
) (bool, error) {
	envelope, err := event.NewWorkspaceEvent(r.IDs.NewID(), eventType, state, actor, at, extra)
	if err != nil {
		return false, err
	}
	return r.Events.Append(ctx, envelope, key)
}

// StateOf is what an event reports of a workspace's row.
func StateOf(record adminrepo.TenantRecord) event.WorkspaceState {
	return event.WorkspaceState{
		ID: record.ID, ExternalReference: record.ExternalReference, Status: string(record.Status),
		Plan: record.PlanName, Version: int64(record.Version),
	}
}
