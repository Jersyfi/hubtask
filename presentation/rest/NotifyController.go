// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// notifiedResponse is a notification step's answer: how many recipients a message was recorded
// for (UC-AUT-09).
func notifiedResponse(out usecase.Output) openapi.NotificationsQueued {
	return openapi.NotificationsQueued{Recipients: out.Int("recipients")}
}
