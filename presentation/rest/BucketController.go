// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The columns of a collection's board. The handlers hold no rules: the permission check, the
// uniqueness of the name, the rank the column lands at and the four records a write owes all happen
// in the application layer, once, whichever channel the call came through (ADR-0005, arc42 §4).

// Creating, listing, reordering and deleting a bucket are answered by the generated mapping
// (Doors.gen.go); an update is written here because a null limit means "no limit", the number 0.
const updateBucketUseCase = "UpdateBucket"

// bucketResponse maps the catalogue's answer onto the contract's Bucket.
//
// `wip_limit` and `color_token` are written whether or not they are set, as explicit nulls: they are
// values a board renders, and a field that appeared only once somebody had set it is one a client
// cannot read unconditionally.
func bucketResponse(out usecase.Output) openapi.Bucket {
	isDoneBucket, _ := out["is_done_bucket"].(bool)

	bucket := openapi.Bucket{
		Id:           uuidValue(out.String("id")),
		CollectionId: uuidValue(out.String("collection_id")),
		Name:         out.String("name"),
		OrderKey:     out.String("order_key"),
		IsDoneBucket: isDoneBucket,
		Version:      out.Int("version"),
	}
	if limit, ok := out["wip_limit"].(int); ok {
		bucket.WipLimit = &limit
	}
	if token := out.String("color_token"); token != "" {
		bucket.ColorToken = &token
	}
	return bucket
}

// UpdateBucket answers PATCH /containers/{containerId}/buckets/{bucketId}.
//
// The collection in the path is not passed on. The column already knows which board it is on, and a
// path segment that could disagree with the row would be a second answer to a question that has one
// - the route names it so that a client can address a column without holding a flat identifier
// space in its head.
func (c *RestController) UpdateBucket(
	w http.ResponseWriter, r *http.Request,
	_ openapi.ContainerId, bucketID openapi.BucketId, _ openapi.UpdateBucketParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.BucketUpdate
	present, err := decodeJSONWithPresence(r, &body)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"bucket_id": bucketID.String()}
	// Only the fields the client sent. A merge patch says "leave it alone" by omission, and a
	// handler that passed every field would clear the colour of every client that only meant to
	// rename something.
	if present["name"] {
		// Null is not an instruction here: the contract types `name` as a plain string, a column
		// without one cannot exist, and the catalogue refuses an empty one by name.
		in["name"] = stringOrEmpty(body.Name)
	}
	if present["color_token"] {
		// Null and the empty string are one instruction: clear it. That is what makes them a
		// different request from omitting the field, which leaves the colour alone.
		in["color_token"] = stringOrEmpty(body.ColorToken)
	}
	if present["wip_limit"] {
		// Null and 0 are the same instruction - remove the limit - because zero is not a limit
		// anybody could satisfy. Reading them as one saves the layers below a second flag beside
		// the number.
		limit := 0
		if body.WipLimit != nil {
			limit = *body.WipLimit
		}
		in["wip_limit"] = limit
	}
	if present["is_done_bucket"] && body.IsDoneBucket != nil {
		in["is_done_bucket"] = *body.IsDoneBucket
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), updateBucketUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	bucket := bucketResponse(out)
	w.Header().Set("ETag", etag(bucket.Version))
	writeJSON(w, r, http.StatusOK, bucket)
}
