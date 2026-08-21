// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// schemaTypes are the types this schema defines itself, which the driver cannot know by OID: an
// enum gets its OID when the migration creates it, so it differs per database.
//
// A scalar enum needs no entry. pgx decodes an unknown OID as text and sqlc maps such a column to
// a string kind, so item_type on its own has always worked. An array of one does not: the array
// codec has to know its element type, and without it the scan fails at run time with "cannot scan
// unknown type ... into []ItemType" - the column reads fine in psql and only the Go side breaks.
// Two columns are affected today, item_capability_profile.allowed_child_types and
// custom_field_definition.applies_to, and the next enum array would fail the same way.
//
// The array form is named explicitly rather than inferred: LoadTypes resolves a type's
// dependencies downwards, from the array to its element, never from the element up to the array.
var schemaTypes = []string{"item_type", "_item_type"}

// RegisterSchemaTypes teaches one connection the types above.
//
// It is exported because a type map belongs to a connection, so every pool has to install it, and
// the integration tests open their own pools against the container rather than going through
// NewPool. Wiring it in as AfterConnect is what keeps that from being something a caller has to
// remember.
//
// A database that has not been migrated yet carries none of these types. LoadTypes then finds
// nothing, returns nothing, and registers nothing rather than failing - which is what lets the
// migrator open a pool against an empty database.
func RegisterSchemaTypes(ctx context.Context, conn *pgx.Conn) error {
	types, err := conn.LoadTypes(ctx, schemaTypes)
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.type_registration_failed").
			WithCause(fmt.Errorf("loading the schema types: %w", err))
	}
	// LoadTypes documents its result as the input to RegisterTypes. It happens to register into
	// the connection's own map on the way, but relying on that would be relying on an
	// implementation detail; registering is idempotent.
	conn.TypeMap().RegisterTypes(types)
	return nil
}
