// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Reading and cancelling a job are answered by the generated mapping (Doors.gen.go); jobResponse
// draws a job wherever one is answered.

// jobResponse maps the catalogue's answer.
//
// The four optional fields are pointers with no omitempty in the generated type, so a job that has
// none of them carries explicit nulls rather than no fields at all - which is what lets a client
// read `progress` unconditionally and tell "cannot say" from "nothing done yet".
func jobResponse(out usecase.Output) openapi.Job {
	job := openapi.Job{
		JobId:     uuidValue(out.String("job_id")),
		Status:    openapi.JobStatus(out.String("status")),
		CreatedAt: timeValue(out["created_at"]),
	}
	if progress, ok := out["progress"].(float64); ok {
		fraction := float32(progress)
		job.Progress = &fraction
	}
	if url := out.String("result_url"); url != "" {
		job.ResultUrl = &url
	}
	if code := out.String("error_code"); code != "" {
		job.ErrorCode = &code
	}
	if params, ok := out["error_params"].(map[string]string); ok && len(params) > 0 {
		job.ErrorParams = &params
	}
	if code := out.String("outcome_code"); code != "" {
		job.OutcomeCode = &code
	}
	if finished := timeValue(out["finished_at"]); !finished.IsZero() {
		job.FinishedAt = &finished
	}
	return job
}
