// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package stepup is the step-up verifier this build has: none (E-06).
//
// Sessions and multi-factor authentication are `0.6.0`. Until then no installation can ask anybody
// to prove who they are a second time, so the adapter says so rather than pretending, and every
// destructive restore is refused with a code that names the reason.
package stepup

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/stepup"
)

// Unavailable is the honest verifier of an installation that cannot re-authenticate anybody.
//
// It is a type rather than a nil check in the application layer, and that is deliberate: a nil
// port would make "this installation has no step-up" indistinguishable from "somebody forgot to
// wire one up", and the second of those is how a destructive mode ends up permitted by omission.
type Unavailable struct{}

var _ port.Verifier = Unavailable{}

// Available is false, and will be until the release that brings sessions back.
func (Unavailable) Available() bool { return false }

// Satisfied is never asked - the caller checks Available first - and answers no if it is.
func (Unavailable) Satisfied(context.Context, shared.ID, string) (bool, error) { return false, nil }
