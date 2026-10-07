// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A deprecated field says so (versioning-release.md §5, api-guidelines.md "Deprecation").
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
	// ReplacedBy names what takes its place - an operationId or a header - and never says it in
	// words: the manifest is read by programs (ADR-0011).
	ReplacedBy []string
}

// deprecationBodyLimit bounds what is read to look for a deprecated field. The bodies that carry
// one are small configuration documents; a larger body is passed on unread rather than buffered.
const deprecationBodyLimit = 64 << 10

// deprecatedByTemplate indexes the table by the router's template, "POST /api/v1/auth/mfa:disable".
var deprecatedByTemplate = func() map[string][]deprecatedField {
	index := map[string][]deprecatedField{}
	for _, field := range deprecatedFields {
		template := field.Method + " " + APIBasePath + field.Path
		index[template] = append(index[template], field)
	}
	return index
}()

// announceDeprecation sets the headers when the request's body sends a deprecated field of the
// operation it was routed to. The body is read and put back, so the handler reads it as it was.
func announceDeprecation(w http.ResponseWriter, r *http.Request, template string) {
	fields := deprecatedByTemplate[template]
	if len(fields) == 0 || r.Body == nil || r.ContentLength > deprecationBodyLimit {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, deprecationBodyLimit+1))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), r.Body))
	if err != nil || len(raw) > deprecationBodyLimit {
		return
	}
	var sent map[string]json.RawMessage
	if json.Unmarshal(raw, &sent) != nil {
		return // the handler answers a malformed body; this only listens
	}
	for _, field := range fields {
		// Case-insensitive, as the handler's decoder matches it: `Password` is the field too.
		if !sentField(sent, field.Field) {
			continue
		}
		if since, err := time.Parse(time.DateOnly, field.Since); err == nil {
			w.Header().Set("Deprecation", "@"+strconv.FormatInt(since.Unix(), 10))
		}
		if sunset, err := time.Parse(time.DateOnly, field.Sunset); err == nil {
			w.Header().Set("Sunset", sunset.UTC().Format(http.TimeFormat))
		}
		return
	}
}

func sentField(sent map[string]json.RawMessage, name string) bool {
	for key := range sent {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

// deprecationManifest is the table as the manifest answers it. Always a list, empty where nothing is
// deprecated, so a client reads "nothing is going" rather than "this server does not say".
func deprecationManifest() *[]openapi.DeprecatedField {
	listed := make([]openapi.DeprecatedField, 0, len(deprecatedFields))
	for _, field := range deprecatedFields {
		entry := openapi.DeprecatedField{
			OperationId: field.OperationID, Method: field.Method, Path: field.Path, Field: field.Field,
			RemovedIn: field.RemovedIn, ReplacedBy: field.ReplacedBy,
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
