// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// installationPresetResponse draws one of the installation's presets (UC-INS-18) through the
// contract's type: the use case's output carries the contract's names.
func installationPresetResponse(out usecase.Output) openapi.InstallationPreset {
	var preset openapi.InstallationPreset
	_ = throughJSON(out, &preset)
	return preset
}
