// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import ()

// Renaming, the privacy and the lifecycle of a container are answered by the generated mapping
// (Doors.gen.go); the policies and a move are written here. The routes come from the
// specification and are reconciled with the catalogue by the parity test.
const updateContainerPoliciesUseCase = "UpdateContainerPolicies"

const moveContainerUseCase = "MoveContainer"

const reorderContainerUseCase = "ReorderContainer"
