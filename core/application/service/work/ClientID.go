// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// clientIDField is the catalogue's `id` input of a creation an offline client may make: the
// identifier it minted, final at once (offline-sync.md §9 rule 1).
var clientIDField = usecase.Field{
	Name: "id", Kind: usecase.KindID, CallerOnly: true,
	Description: "The identifier the caller minted, a UUIDv7; offline clients assign their own. " +
		"Leave it out and the server mints one.",
}

// mintedOr is the identifier the client assigned, or a fresh one; a client's that is not a UUIDv7
// is refused, because only a time-ordered identifier is final on every device.
func mintedOr(given shared.ID, ids clock.IDGenerator) (shared.ID, error) {
	if given.IsZero() {
		return ids.NewID(), nil
	}
	if !given.IsUUIDv7() {
		return "", shared.ErrValidation.
			WithDetail("sync.id_not_uuidv7").
			WithFields(shared.FieldError{Path: "/id", Code: "sync.id_not_uuidv7"})
	}
	return given, nil
}
