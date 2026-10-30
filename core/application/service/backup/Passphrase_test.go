// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"context"
	"errors"
	"testing"

	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A passphrase this build does not serve is refused rather than ignored, on every door, before
// anything is written: one with no effect would leave somebody believing their archives are
// protected by it.
func TestAnEncryptionPassphraseIsRefusedRatherThanIgnored(t *testing.T) {
	handler := service.CreateBackupTarget{}.Descriptor().Handler
	_, err := handler.Invoke(context.Background(), appshared.ActorContext{}, usecase.Input{
		"name": "Off-site bucket", "kind": "S3", "config": map[string]any{"bucket": "b"},
		"encryption_passphrase": "correct horse battery staple",
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
	if service.RefusePassphrase(usecase.Input{"encryption_passphrase": ""}) != nil {
		t.Fatal("an empty passphrase is no passphrase")
	}
}

// A restore's passphrase is refused the same way, before any archive is read.
func TestARestorePassphraseIsRefusedRatherThanIgnored(t *testing.T) {
	handler := service.StartRestore{}.Descriptor().Handler
	_, err := handler.Invoke(context.Background(), appshared.ActorContext{}, usecase.Input{
		"target_id": "0192f000-0000-7000-8000-0000000000c1", "archive_id": "a", "mode": "MERGE",
		"decryption_passphrase": "correct horse battery staple",
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
}
