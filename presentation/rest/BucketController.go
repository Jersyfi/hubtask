// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
