// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The columns of a collection's board (B-09). The handlers hold no rules: the permission check, the
// uniqueness of the name, the rank the column lands at and the four records a write owes all happen
// in the application layer, once, whichever channel the call came through (ADR-0005, arc42 §4).

const (
	createBucketUseCase  = "CreateBucket"
	listBucketsUseCase   = "ListBuckets"
	updateBucketUseCase  = "UpdateBucket"
	reorderBucketUseCase = "ReorderBucket"
)

// CreateBucket answers POST /containers/{containerId}/buckets.
func (c *RestController) CreateBucket(
	w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.BucketCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"collection_id": containerID.String(),
		"name":          body.Name,
		"wip_limit":     optionalIntField(body.WipLimit),
		"color_token":   optionalStringField(body.ColorToken),
	}
	if body.IsDoneBucket != nil {
		in["is_done_bucket"] = *body.IsDoneBucket
	}
	if body.BeforeBucketId != nil {
		in["before_bucket_id"] = body.BeforeBucketId.String()
	}

	out, err := c.UseCases.Invoke(r.Context(), createBucketUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	bucket := bucketResponse(out)
	// Location and ETag are what let a client follow up without guessing: where the column is, and
	// which version it may write against (api-guidelines.md §5).
	w.Header().Set("Location",
		APIBasePath+"/containers/"+containerID.String()+"/buckets/"+bucket.Id.String())
	w.Header().Set("ETag", etag(bucket.Version))
	writeJSON(w, r, http.StatusCreated, bucket)
}

// ListBuckets answers GET /containers/{containerId}/buckets.
//
// A plain array rather than a page, as the contract declares: a board has as many columns as fit on
// a screen (api-guidelines.md §2).
func (c *RestController) ListBuckets(
	w http.ResponseWriter, r *http.Request, containerID openapi.ContainerId,
) {
	out, ok := c.read(w, r, listBucketsUseCase, usecase.Input{
		"collection_id": containerID.String(),
	})
	if !ok {
		return
	}

	board := []openapi.Bucket{}
	for _, row := range rowsOf(out) {
		board = append(board, bucketResponse(row))
	}
	writeJSON(w, r, http.StatusOK, board)
}

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
	if present["name"] && body.Name != nil {
		in["name"] = *body.Name
	}
	if present["color_token"] {
		in["color_token"] = optionalStringField(body.ColorToken)
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

// ReorderBucket answers POST /containers/{containerId}/buckets/{bucketId}:reorder.
func (c *RestController) ReorderBucket(
	w http.ResponseWriter, r *http.Request,
	_ openapi.ContainerId, bucketID openapi.BucketId, _ openapi.ReorderBucketParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	in := usecase.Input{"bucket_id": bucketID.String()}

	// The body is optional: no body at all means "move it to the right hand end", which is a
	// position like any other.
	if r.ContentLength > 0 {
		var body openapi.ReorderBucketJSONBody
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, requestID)
			return
		}
		if body.BeforeBucketId != nil {
			in["before_bucket_id"] = body.BeforeBucketId.String()
		}
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), reorderBucketUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	bucket := bucketResponse(out)
	w.Header().Set("ETag", etag(bucket.Version))
	writeJSON(w, r, http.StatusOK, bucket)
}
