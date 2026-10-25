// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

type store struct{}

func (store) Find(context.Context) (domain.Settings, error)                     { return domain.DefaultSettings(), nil }
func (store) Save(context.Context, domain.Settings, shared.ID, time.Time) error { return nil }
func (store) WorkspaceNamed(context.Context, string) (shared.ID, error)         { return "", nil }
func (store) Seen(context.Context, string) (bool, error)                        { return false, nil }
func (store) Record(context.Context, domain.Receipt) (bool, error)              { return true, nil }
func (store) CountFrom(context.Context, string, time.Time) (int, error)         { return 0, nil }
func (store) Recent(context.Context, int) ([]domain.Receipt, error)             { return nil, nil }
func (store) DeleteExpired(context.Context, time.Time, int) (int, error)        { return 0, nil }
func (store) CountExpired(context.Context, time.Time, int) (int, error)         { return 0, nil }
func (store) CollectionByHandle(context.Context, string) (shared.ID, error)     { return "", nil }
func (store) ItemByKey(context.Context, domain.Key) (shared.ID, error)          { return "", nil }

// The ports are small enough that one value can be all three, which is how the adapter is built.
func TestOneStoreCanServeEveryPort(t *testing.T) {
	var (
		_ mail.Settings = store{}
		_ mail.Receipts = store{}
		_ mail.Places   = store{}
	)
	if settings, _ := (store{}).Find(context.Background()); settings.Enabled {
		t.Fatal("a store without settings answers the intake off")
	}
}

func (store) KeyOf(context.Context, shared.ID) (domain.Key, error) { return domain.Key{}, nil }
func (store) SetHandle(context.Context, shared.ID, string) error   { return nil }

type boxes struct{}

func (boxes) List(context.Context) ([]domain.Mailbox, error) { return nil, nil }
func (boxes) Find(context.Context, shared.ID) (domain.Mailbox, crypto.Sealed, error) {
	return domain.Mailbox{}, crypto.Sealed{}, nil
}
func (boxes) Insert(context.Context, domain.Mailbox, crypto.Sealed, shared.ID) error { return nil }
func (boxes) RecordCheck(context.Context, domain.Mailbox) error                      { return nil }
func (boxes) Delete(context.Context, shared.ID) error                                { return nil }
func (boxes) Count(context.Context) (int, error)                                     { return 0, nil }

func TestTheMailboxStoreIsOnePort(t *testing.T) {
	var store mail.Mailboxes = boxes{}
	if count, err := store.Count(context.Background()); count != 0 || err != nil {
		t.Fatal(count, err)
	}
}
