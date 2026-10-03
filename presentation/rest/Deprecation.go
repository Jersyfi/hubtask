// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A deprecated field says so (SC-28, versioning-release.md §5, api-guidelines.md "Deprecation").
//
// The contract marks a request field `deprecated` with the day it was, the major version it goes
// away with and, once one is set, the day it stops being accepted. `tools/deprecations` reads that
// into `deprecatedFields` at `make generate`, and two things are answered from it: the manifest's
// `deprecations`, so a client can read what is going before it goes, and - on a request that
// actually sends such a field - the `Deprecation` header (RFC 9745) and, where a day is set,
// `Sunset` (RFC 8594). A request that does not send it hears nothing: the header says "what you
// just did is going away", not "this route has something deprecated somewhere".

// deprecatedField is one row of the generated table.
type deprecatedField struct {
	OperationID string
	Method      string
	// Path is the operation's path as the specification writes it, without the base path.
	Path      string
	Field     string
	Since     string
	RemovedIn string
	Sunset    string
	Reason    string
}

// deprecationManifest is the table as the manifest answers it. Always a list, empty where nothing is
// deprecated, so a client reads "nothing is going" rather than "this server does not say".
func deprecationManifest() *[]openapi.DeprecatedField {
	listed := make([]openapi.DeprecatedField, 0, len(deprecatedFields))
	for _, field := range deprecatedFields {
		entry := openapi.DeprecatedField{
			OperationId: field.OperationID, Method: field.Method, Path: field.Path, Field: field.Field,
			RemovedIn: field.RemovedIn, Reason: field.Reason,
		}
		if since, err := time.Parse(time.DateOnly, field.Since); err == nil {
			entry.Since = openapi_types.Date{Time: since}
		}
		if sunset, err := time.Parse(time.DateOnly, field.Sunset); err == nil {
			entry.Sunset = &openapi_types.Date{Time: sunset}
		}
		listed = append(listed, entry)
	}
	return &listed
}
