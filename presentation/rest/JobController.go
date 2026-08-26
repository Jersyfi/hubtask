// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	getJobUseCase    = "GetJob"
	cancelJobUseCase = "CancelJob"
)

// GetJob answers GET /jobs/{jobId}.
//
// The resource a `202 Accepted` has been pointing at since A-06. Nothing here decides what a
// caller may see: the use case asks that question, and a job in another tenant reaches this
// method as the same not-found the catalogue answers for one that never existed.
func (c *RestController) GetJob(w http.ResponseWriter, r *http.Request, jobID openapi.JobId) {
	out, ok := c.read(w, r, getJobUseCase, usecase.Input{"job_id": jobID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, jobResponse(out))
}

// CancelJob answers POST /jobs/{jobId}:cancel.
//
// 200 with the job rather than 204, because what a caller wants next is the state it is now in -
// and because a cancellation is cooperative: the answer says CANCELLED, and the pass that was
// under way is still winding itself up.
func (c *RestController) CancelJob(
	w http.ResponseWriter, r *http.Request, jobID openapi.JobId, _ openapi.CancelJobParams,
) {
	out, ok := c.read(w, r, cancelJobUseCase, usecase.Input{"job_id": jobID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, jobResponse(out))
}

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
	if finished := timeValue(out["finished_at"]); !finished.IsZero() {
		job.FinishedAt = &finished
	}
	return job
}
