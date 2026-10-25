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
