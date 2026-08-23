// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"

	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// ResilientStore composes an ObjectStore with the A-05 blocks (ADR-0016): a bulkhead so the
// storage cannot take more of the process than its pool, and a breaker so a dead endpoint costs
// an immediate answer instead of a blocked thread.
//
// No timeout of its own, and that is deliberate rather than missing (rule 7 is kept elsewhere):
// every call arrives with the caller's deadline - the API path's request deadline, a worker's
// job deadline - and a 64 MiB object through a slow link has no single number this wrapper could
// choose without breaking somebody's legitimate upload. What can hang without progress is
// bounded in the adapter's transport: dial, TLS, and the wait for headers.
type ResilientStore struct {
	inner    port.ObjectStore
	breaker  *resilience.Breaker
	bulkhead *resilience.Bulkhead
}

// NewResilientStore wraps the store. One breaker and one bulkhead per dependency, owned by the
// composition root - the health probe reads the same breaker.
func NewResilientStore(
	inner port.ObjectStore, breaker *resilience.Breaker, bulkhead *resilience.Bulkhead,
) ResilientStore {
	return ResilientStore{inner: inner, breaker: breaker, bulkhead: bulkhead}
}

var _ port.ObjectStore = ResilientStore{}

func (r ResilientStore) Put(ctx context.Context, upload port.Upload) error {
	return r.guarded(ctx, func(ctx context.Context) error {
		return r.inner.Put(ctx, upload)
	})
}

func (r ResilientStore) Get(ctx context.Context, key string) (port.Object, error) {
	var object port.Object
	err := r.guarded(ctx, func(ctx context.Context) error {
		var err error
		object, err = r.inner.Get(ctx, key)
		return err
	})
	if err != nil {
		return port.Object{}, err
	}
	return object, nil
}

func (r ResilientStore) Delete(ctx context.Context, key string) error {
	return r.guarded(ctx, func(ctx context.Context) error {
		return r.inner.Delete(ctx, key)
	})
}

// guarded is the composition, bulkhead outside the breaker: a call that cannot get a slot must
// not count against the breaker - a full pool is this process's state, not the dependency's.
func (r ResilientStore) guarded(ctx context.Context, fn func(context.Context) error) error {
	return r.bulkhead.Do(ctx, func(ctx context.Context) error {
		return r.breaker.Do(ctx, fn)
	})
}
