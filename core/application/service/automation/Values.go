// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import "github.com/Jersyfi/hubtask/core/application/condition"

// Entries and Containers are the reads an expression's activation makes, declared where the
// activation lives (core/application/condition) since G-09: the outbound sender renders a body
// template against the same names a condition reads, and one activation is what keeps the two
// from disagreeing.
type Entries = condition.Entries

// Containers is the container half of the same contract.
type Containers = condition.Containers
