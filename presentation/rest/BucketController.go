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
	createBucketUseCase = "CreateBucket"
	listBucketsUseCase  = "ListBuckets"
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
