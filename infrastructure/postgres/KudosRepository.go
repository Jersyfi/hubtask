// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// KudosRepository keeps kudos (db/queries/Kudos.sql). Every read is one person's own side.
type KudosRepository struct{}

func NewKudosRepository() KudosRepository { return KudosRepository{} }

var _ repository.Kudos = KudosRepository{}

func kudosFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// kudosKey is the three identifiers every statement names.
func kudosKey(itemID, givenBy, givenTo shared.ID) (item, by, to pgtype.UUID, err error) {
	if item, err = uuidOf(itemID); err != nil {
		return
	}
	if by, err = uuidOf(givenBy); err != nil {
		return
	}
	to, err = uuidOf(givenTo)
	return
}

// Give records the kudos; kudos already given keep their moment.
func (KudosRepository) Give(ctx context.Context, kudos work.Kudos) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	item, by, to, err := kudosKey(kudos.ItemID, kudos.GivenBy, kudos.GivenTo)
	if err != nil {
		return false, err
	}
	rows, err := queries.GiveKudos(ctx, sqlc.GiveKudosParams{
		ItemID: item, GivenBy: by, GivenTo: to, GivenAt: timestampOf(kudos.GivenAt),
	})
	if err != nil {
		return false, kudosFailed("giving kudos", err)
	}
	return rows > 0, nil
}

// TakeBack removes the giver's kudos on this completion; none given is no error.
func (KudosRepository) TakeBack(ctx context.Context, itemID, givenBy, givenTo shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	item, by, to, err := kudosKey(itemID, givenBy, givenTo)
	if err != nil {
		return false, err
	}
	rows, err := queries.TakeBackKudos(ctx, sqlc.TakeBackKudosParams{ItemID: item, GivenBy: by, GivenTo: to})
	if err != nil {
		return false, kudosFailed("taking back kudos", err)
	}
	return rows > 0, nil
}

// Given reports whether the giver gave kudos to this completion.
func (KudosRepository) Given(ctx context.Context, itemID, givenBy, givenTo shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	item, by, to, err := kudosKey(itemID, givenBy, givenTo)
	if err != nil {
		return false, err
	}
	given, err := queries.KudosGiven(ctx, sqlc.KudosGivenParams{ItemID: item, GivenBy: by, GivenTo: to})
	if err != nil {
		return false, kudosFailed("reading kudos given", err)
	}
	return given, nil
}

// Received answers the kudos the person received on the entry, oldest first.
func (KudosRepository) Received(ctx context.Context, itemID, givenTo shared.ID) ([]work.Kudos, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	to, err := uuidOf(givenTo)
	if err != nil {
		return nil, err
	}
	rows, err := queries.KudosReceived(ctx, sqlc.KudosReceivedParams{ItemID: item, GivenTo: to})
	if err != nil {
		return nil, kudosFailed("reading kudos received", err)
	}
	received := make([]work.Kudos, 0, len(rows))
	for _, row := range rows {
		givenBy, err := idFrom(row.GivenBy)
		if err != nil {
			return nil, err
		}
		received = append(received, work.Kudos{
			ItemID: itemID, GivenBy: givenBy, GivenTo: givenTo, GivenAt: timeFrom(row.GivenAt),
		})
	}
	return received, nil
}
