// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// IssueSetupCode is the first start's half of UC-INS-01: on an installation with no workspace and
// nobody in its register, a process that serves the API draws a one-time code, stores its hash for
// thirty minutes, and hands the code back to be shown on the console. On any other installation it
// issues nothing.
//
// Not in the registry, and so on no route: the code exists to be read where the server's own output
// is readable and nowhere else. Each process issues its own; any unexpired one sets the
// installation up, and the claim lets only the first setup through.
type IssueSetupCode struct {
	Codes      repository.SetupCodes
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
}

// IssuedSetupCode is the code to print and the moment it stops working.
type IssuedSetupCode struct {
	Code      domain.SetupCode
	ExpiresAt time.Time
}

// Execute issues a code where the installation is empty. The boolean is false where it is not.
func (i IssueSetupCode) Execute(ctx context.Context) (IssuedSetupCode, bool, error) {
	material, err := i.Entropy.Bytes(domain.SetupCodeBytes)
	if err != nil {
		return IssuedSetupCode{}, false, shared.ErrInternal.
			WithDetail("setup.code_unmintable").WithCause(err)
	}
	code, err := domain.NewSetupCode(material)
	if err != nil {
		return IssuedSetupCode{}, false, err
	}

	now := i.Clock.Now()
	issued := IssuedSetupCode{Code: code, ExpiresAt: now.Add(domain.SetupCodeLifetime).UTC()}
	var landed bool
	// The system scope: the table belongs to no workspace, and on an empty installation there is
	// none to be in.
	err = i.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		ok, err := i.Codes.Issue(ctx, i.IDs.NewID(), code, now, issued.ExpiresAt)
		landed = ok
		return err
	})
	if err != nil || !landed {
		return IssuedSetupCode{}, false, err
	}
	return issued, true, nil
}
