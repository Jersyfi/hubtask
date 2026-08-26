// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backup is the outbound port for the targets a tenant has configured (E-03).
package backup

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Targets stores and reads what a tenant has configured, and keeps the credential apart from
// everything else.
//
// Credentials is a method of its own rather than a field on a target, and that is the shape the
// whole "never returned by any read" requirement rests on: the rows that go to a response cannot
// contain a credential, because the statements behind them do not select one. A field that a
// mapper could accidentally copy is a field that eventually is.
type Targets interface {
	// Insert writes the target and its sealed credential. The credential arrives already sealed:
	// this port knows there is one and never what it says.
	Insert(ctx context.Context, target domain.Target, credential crypto.Sealed) error

	// List answers the tenant's targets, by name. Never a credential.
	List(ctx context.Context) ([]domain.Target, error)

	// Find answers one target, or ErrNotFound. Never a credential.
	Find(ctx context.Context, id shared.ID) (domain.Target, error)

	// Credential answers the sealed credential of a target, still sealed. The only caller is the
	// use case that opens a connection, and the only thing it does with the answer is hand it to
	// the encryptor.
	Credential(ctx context.Context, id shared.ID) (crypto.Sealed, error)

	// RecordTest writes down what the connection probe found: when, whether it worked, and the
	// message code when it did not. Never a driver message - one of those carries the host, the
	// user and sometimes the password (rule 10).
	RecordTest(ctx context.Context, id shared.ID, at time.Time, ok bool, code string) error

	// Coverage is what the installation's health surface asks: how many targets there are, and
	// how many of them store an archive unencrypted (backup-restore.md §10).
	Coverage(ctx context.Context) (Coverage, error)
}

// Coverage is the answer to "is this tenant backed up, and how badly".
type Coverage struct {
	Configured  int
	Unencrypted int
}
