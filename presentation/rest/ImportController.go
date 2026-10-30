// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The imports. The controller holds no rules: which hub, which file and which kind are
// acceptable is decided inwards of here. What is decided here is the shape of the answer - a
// job pointer whose result_url names the import, and the import read back as the contract's
// ImportRun.

const ()

// importJobResponse maps an accepted import: the job and where its report will be.
func importJobResponse(out usecase.Output) openapi.JobRef {
	resultURL := out.String("result_url")
	return openapi.JobRef{
		JobId:     uuidValue(out.String("job_id")),
		Status:    openapi.JobStatusQUEUED,
		ResultUrl: &resultURL,
	}
}

func importRunResponse(out usecase.Output) openapi.ImportRun {
	run := openapi.ImportRun{
		Id:        uuidValue(out.String("id")),
		Kind:      openapi.ImportKind(out.String("kind")),
		HubId:     uuidValue(out.String("hub_id")),
		Status:    openapi.ImportRunStatus(out.String("status")),
		CreatedAt: timeValue(out["created_at"]),
	}
	if collection := out.String("collection_id"); collection != "" {
		id := uuidValue(collection)
		run.CollectionId = &id
	}
	if media := out.String("media_id"); media != "" {
		id := uuidValue(media)
		run.MediaId = &id
	}
	if at, ok := out["finished_at"].(time.Time); ok && !at.IsZero() {
		run.FinishedAt = &at
	}
	if code := out.String("error_code"); code != "" {
		run.ErrorCode = &code
	}
	if report, ok := out["report"].(usecase.Output); ok {
		run.Report = restoreReportResponse(report)
	}
	if refused, ok := out["refused"].([]usecase.Output); ok {
		rows := make([]struct {
			Code string `json:"code"`
			Row  int    `json:"row"`
		}, 0, len(refused))
		for _, each := range refused {
			rows = append(rows, struct {
				Code string `json:"code"`
				Row  int    `json:"row"`
			}{Code: each.String("code"), Row: each.Int("row")})
		}
		run.Refused = &rows
	}
	return run
}
