// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"log/slog"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// scanBatch bounds one pass: each file is read whole through the scanner.
const scanBatch = 50

// ScanHeldMedia settles the uploads held while the scanner could not answer (queue.KindMediaScan):
// clean ones become READY, malicious ones INFECTED with their bytes removed at once. Internal, for
// ReconcileMedia's reason - nobody asks for it; a held confirmation does.
//
// The scanner is called outside any transaction (observability-reliability.md §8), and a pass
// stops at the first file it still cannot answer for: the scanner is down for all of them. A file
// the scanner refuses for itself (media.TooLargeToScan) is settled UNSCANNABLE and the pass goes
// on - held, the oldest file would stop every pass behind it for good.
type ScanHeldMedia struct {
	Objects    repository.Objects
	Store      storage.ObjectStore
	Scanner    storage.Scanner
	UnitOfWork persistence.UnitOfWork
}

// ScanOutcome is what one pass did, and what it left held.
type ScanOutcome struct {
	Settled int
	Held    int
}

// Execute runs one pass for the tenant.
func (h ScanHeldMedia) Execute(ctx context.Context, tenantID shared.ID) (ScanOutcome, error) {
	scope := persistence.Scope{TenantID: tenantID}
	var held []media.Object
	if err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		held, err = h.Objects.Held(ctx, scanBatch)
		return err
	}); err != nil {
		return ScanOutcome{}, err
	}
	if h.Scanner == nil {
		// Scanning was switched off with files held: they stay held, never served unscanned.
		return ScanOutcome{Held: len(held)}, nil
	}

	outcome := ScanOutcome{}
	for i, object := range held {
		verdict, err := h.scanOne(ctx, object)
		if errors.Is(err, shared.ErrNotFound) {
			// No bytes to scan, so none to serve: the reconciliation's business, not this pass's.
			continue
		}
		if errors.Is(err, storage.ErrNoVerdictForFile) {
			// The scanner is up and answered an error for this file: it stays held, and the files
			// behind it are not stuck behind it.
			slog.WarnContext(ctx, "the upload scanner answered an error for a held file; it stays held",
				slog.String("media_id", object.ID.String()))
			outcome.Held++
			continue
		}
		if errors.Is(err, shared.ErrUnavailable) {
			outcome.Held += len(held) - i
			return outcome, nil
		}
		var settled media.Object
		switch {
		case media.IsTooLargeToScan(err):
			slog.WarnContext(ctx, "the upload scanner refused a held file for its size; it is never served",
				slog.String("media_id", object.ID.String()), slog.Int64("byte_size", object.ByteSize))
			settled, err = object.Unscannable()
		case err != nil:
			return outcome, err
		default:
			settled, err = object.Scanned(verdict.Malicious)
		}
		if err != nil {
			return outcome, err
		}
		if verdict.Malicious {
			slog.WarnContext(ctx, "a held upload was found malicious; its bytes are removed",
				slog.String("media_id", object.ID.String()), slog.String("signature", verdict.Signature))
			if err := h.Store.Delete(ctx, object.StorageKey); err != nil {
				// Still held, so still never served: the next pass tries again.
				outcome.Held++
				continue
			}
		}
		err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			return h.Objects.Settle(ctx, settled)
		})
		if err != nil && !errors.Is(err, shared.ErrConflict) {
			return outcome, err
		}
		outcome.Settled++
	}
	if len(held) == scanBatch {
		// A full batch: there may be more behind it.
		outcome.Held++
	}
	return outcome, nil
}

func (h ScanHeldMedia) scanOne(ctx context.Context, object media.Object) (storage.Verdict, error) {
	stored, err := h.Store.Get(ctx, object.StorageKey)
	if err != nil {
		return storage.Verdict{}, err
	}
	defer func() { _ = stored.Content.Close() }()
	return h.Scanner.Scan(ctx, stored.Content)
}

// seedScan asks for the tenant's scan job; a build wired without a queue asks nothing.
func seedScan(ctx context.Context, jobs Enqueuer, tenantID shared.ID) error {
	if jobs == nil {
		return nil
	}
	_, err := jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindMediaScan, TenantID: tenantID, DedupeKey: tenantID.String(),
	})
	return err
}
