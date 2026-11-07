// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The reads a person meets before asking AI anything: which entries look like the title being
// typed (UC-AI-04 check 7).

// aiBudgetStateResponse writes the moment as null for OFF rather than leaving it out, which the
// generated type's omitempty would.
func aiBudgetStateResponse(out usecase.Output) map[string]any {
	return map[string]any{"state": out["state"], "renews_at": out["renews_at"]}
}
