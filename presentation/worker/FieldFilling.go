// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// FieldFilling is the job of queue.KindFieldFill: one page of a new CALCULATED field's fill (the
// use case enqueues the next page's job until the walk reaches the end).
type FieldFilling struct {
	Filling work.FillCalculatedFields
}

var _ queue.Handler = FieldFilling{}

func (f FieldFilling) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("fields.fill_without_tenant")
	}
	definition, _ := job.Payload["definition_id"].(string)
	collection, _ := job.Payload["collection_id"].(string)
	after, _ := job.Payload["after"].(string)
	cmd := work.FillCalculatedFieldsCommand{
		DefinitionID: shared.ID(definition), CollectionID: shared.ID(collection), After: shared.ID(after),
	}
	outcome, err := f.Filling.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}, cmd)
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{Counts: map[string]int{"filled": outcome.Filled}}, nil
}
