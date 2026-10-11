// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The temporary cluster is read on one connection whose session cannot write and whose statements
// end at the read's own bound.
func TestTheTemporaryClusterIsReadOnOneReadOnlyBoundedConnection(t *testing.T) {
	cfg, err := recoveredHoldsSource("postgres://hubtask_app@recovered.example:5432/hubtask", 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	params := cfg.ConnConfig.RuntimeParams
	if params["default_transaction_read_only"] != "on" {
		t.Errorf("default_transaction_read_only = %q", params["default_transaction_read_only"])
	}
	if params["statement_timeout"] != "90000" {
		t.Errorf("statement_timeout = %q", params["statement_timeout"])
	}
	if cfg.MaxConns != 1 || cfg.ConnConfig.ConnectTimeout != 90*time.Second {
		t.Errorf("max %d connections, connect timeout %s", cfg.MaxConns, cfg.ConnConfig.ConnectTimeout)
	}
}

// A connection string the driver cannot read is a refusal the operator can correct, not an outage.
func TestAnUnreadableConnectionStringIsRefused(t *testing.T) {
	if _, err := recoveredHoldsSource("postgres://owner@[broken", time.Minute); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
}

// A read without a workspace is refused before anything connects.
func TestReadingRecoveredHoldsNeedsAWorkspace(t *testing.T) {
	_, err := ReadRecoveredHolds(context.Background(), "postgres://nowhere.invalid/x", "", time.Second)
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("err = %v", err)
	}
}
