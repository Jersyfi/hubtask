// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// shapeCustomFields reads the definitions behind every custom field a query names and puts their
// shape on it (F-2): which typed column a range, a sort or a grouping reads. Inside the caller's
// read transaction, because the definitions are tenant rows. A scope is a collection or a hub,
// zero for the whole workspace.
func shapeCustomFields(
	ctx context.Context, fields repository.CustomFields, spec view.Spec, scope shared.ID,
) (view.Spec, error) {
	keys := spec.CustomKeys()
	shapes := make(map[string]view.CustomShape, len(keys))
	if len(keys) > 0 {
		if fields == nil {
			return view.Spec{}, shared.ErrInternal.WithDetail("query.custom_fields_unwired")
		}
		kinds, err := fields.KindsByKey(ctx, keys, scope)
		if err != nil {
			return view.Spec{}, err
		}
		for _, key := range keys {
			shapes[key] = view.ShapeOf(kinds[key])
		}
	}
	return spec.Shaped(shapes)
}
