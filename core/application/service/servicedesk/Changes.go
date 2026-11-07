// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The change log's names for the desk's records about an entry, each filed under the entry's
// collection: whoever reads the entry reads its approvals, its goals and its score (offline-sync.md
// §3.1).
const (
	ApprovalEntity     = "approval"
	EntrySLAEntity     = "entry_sla"
	SatisfactionEntity = "satisfaction_rating"
)

// ApprovalRecord is an approval as the change log carries it: the approval as every door answers
// it, with nobody's leave to decide - who may decide is the server's question, asked online.
func ApprovalRecord(approval domain.Approval, decisions []domain.Decision) usecase.Output {
	return ApprovalOutput(approval, decisions, false)
}

// SLARecord is one goal's standing on an entry as the change log carries it.
func SLARecord(record domain.EntrySLA) usecase.Output {
	out := RecordOutput(record)
	out["id"], out["item_id"] = record.ID.String(), record.ItemID.String()
	return out
}

// RatingRecord is a request's score as the change log carries it.
func RatingRecord(rating domain.Rating) usecase.Output {
	return usecase.Output{
		"id": rating.ID.String(), "item_id": rating.ItemID.String(), "score": rating.Score,
		"rated_at": rating.RatedAt.UTC(),
	}
}

// Announcer tells the change log of every approval, SLA record and rating the desk writes, inside
// the writer's transaction, by wrapping the stores the use cases and the clocks already write
// through: one place, so that no writer can forget it.
type Announcer struct {
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
	Items   workrepo.Items
}

// announce records the row under its entry's collection; an entry gone takes its rows with it.
func (a Announcer) announce(ctx context.Context, tenant, itemID shared.ID, entity string, id shared.ID, payload usecase.Output) error {
	item, err := a.Items.Find(ctx, itemID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return a.Changes.Record(ctx, changelog.Change{
		TenantID: tenant, Entity: entity, EntityID: id, Op: changelog.Upsert,
		ContainerID: item.CollectionID, HLC: a.HLC.Next(), Payload: payload,
	})
}

// Records wraps the SLA records' store.
func (a Announcer) Records(store repository.Records) repository.Records {
	return announcedRecords{Records: store, announcer: a}
}

// Ratings wraps the ratings' store.
func (a Announcer) Ratings(store repository.Ratings) repository.Ratings {
	return announcedRatings{Ratings: store, announcer: a}
}

// Approvals wraps the approvals' store.
func (a Announcer) Approvals(store repository.Approvals) repository.Approvals {
	return announcedApprovals{Approvals: store, announcer: a}
}

type announcedRecords struct {
	repository.Records
	announcer Announcer
}

// Save announces the record as the store now holds it - by its goal, since a goal is armed once.
func (r announcedRecords) Save(ctx context.Context, record domain.EntrySLA, now time.Time) error {
	if err := r.Records.Save(ctx, record, now); err != nil {
		return err
	}
	held, err := r.OfItem(ctx, record.ItemID)
	if err != nil {
		return err
	}
	for _, stored := range held {
		if stored.Goal == record.Goal {
			return r.announcer.announce(ctx, stored.TenantID, stored.ItemID, EntrySLAEntity, stored.ID, SLARecord(stored))
		}
	}
	return nil
}

type announcedRatings struct {
	repository.Ratings
	announcer Announcer
}

func (r announcedRatings) Save(ctx context.Context, rating domain.Rating) (domain.Rating, error) {
	stored, err := r.Ratings.Save(ctx, rating)
	if err != nil {
		return stored, err
	}
	return stored, r.announcer.announce(ctx, stored.TenantID, stored.ItemID, SatisfactionEntity, stored.ID, RatingRecord(stored))
}

type announcedApprovals struct {
	repository.Approvals
	announcer Announcer
}

func (r announcedApprovals) Insert(ctx context.Context, approval domain.Approval) error {
	if err := r.Approvals.Insert(ctx, approval); err != nil {
		return err
	}
	return r.again(ctx, approval.ID)
}

// Update announces the approval with its trail; a decision is recorded before the approval moves.
func (r announcedApprovals) Update(ctx context.Context, approval domain.Approval, expectedVersion int) error {
	if err := r.Approvals.Update(ctx, approval, expectedVersion); err != nil {
		return err
	}
	return r.again(ctx, approval.ID)
}

func (r announcedApprovals) again(ctx context.Context, id shared.ID) error {
	stored, err := r.Find(ctx, id)
	if err != nil {
		return err
	}
	decisions, err := r.Decisions(ctx, id)
	if err != nil {
		return err
	}
	return r.announcer.announce(ctx, stored.TenantID, stored.ItemID, ApprovalEntity, stored.ID, ApprovalRecord(stored, decisions))
}
