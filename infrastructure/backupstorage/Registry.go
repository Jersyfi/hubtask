// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// Registry is what this build can talk to (ADR-0019 decision 2).
//
// The contract's enum names eleven kinds and this build has four. That difference is answered
// rather than hidden: a target of a kind nobody has written an adapter for is refused with
// `backup.kind_unsupported`, which says "Hubtask cannot talk to SMB yet" - a different sentence
// from "SMB is not a thing", and the one that is true.
//
// The remaining seven ship when they pass BK-1. That is what makes the conformance suite a gate
// rather than an aspiration: an adapter is finished when the same suite that holds these four
// holds it too.
type Registry struct {
	client    *httpclient.GuardedClient
	guard     *httpclient.Guard
	localRoot string
	timeout   time.Duration
	now       func() time.Time
}

// NewRegistry wires the four adapters.
//
// localRoot is the installation's backup volume (HUBTASK_BACKUP_LOCAL_PATH). Empty means this
// installation serves no local targets, which is a valid answer for a deployment whose container
// has no writable volume - and a much better one than writing an archive into an overlay
// filesystem that disappears with the pod.
func NewRegistry(
	client *httpclient.GuardedClient, guard *httpclient.Guard,
	localRoot string, timeout time.Duration, now func() time.Time,
) Registry {
	return Registry{
		client: client, guard: guard, localRoot: localRoot, timeout: timeout, now: now,
	}
}

var _ port.Opener = Registry{}

// Kinds is what a client may offer and what `/meta/capabilities` reports. Never the contract's
// full enum.
func (r Registry) Kinds() []backup.TargetKind {
	return []backup.TargetKind{
		backup.KindLocal, backup.KindS3, backup.KindSFTP, backup.KindWebDAV,
	}
}

// Open turns a stored target into something that can be written to.
func (r Registry) Open(_ context.Context, spec port.Spec) (port.Store, error) {
	switch spec.Kind {
	case backup.KindLocal:
		return NewLocalStore(r.localRoot, spec.Config.Get("path"))
	case backup.KindS3:
		return NewS3Store(r.client, spec, r.now)
	case backup.KindWebDAV:
		return NewWebDAVStore(r.client, spec)
	case backup.KindSFTP:
		return NewSFTPStore(r.guard, spec, r.timeout)
	default:
		return nil, shared.ErrValidation.
			WithDetail(port.CodeKindUnsupported).
			WithParams(map[string]string{"kind": spec.Kind.String()}).
			WithFields(shared.FieldError{
				Path:   "/kind",
				Code:   port.CodeKindUnsupported,
				Params: map[string]string{"kind": spec.Kind.String()},
			})
	}
}
