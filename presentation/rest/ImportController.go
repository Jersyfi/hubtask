// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The imports (P-08). The controller holds no rules: which hub, which file and which kind are
// acceptable is decided inwards of here. What is decided here is the shape of the answer - a
// job pointer whose result_url names the import, and the import read back as the contract's
// ImportRun.

const (
	importEntriesUseCase = "ImportEntries"
	getImportUseCase     = "GetImport"
)

// ImportEntries answers POST /imports.
func (c *RestController) ImportEntries(w http.ResponseWriter, r *http.Request, _ openapi.ImportEntriesParams) {
	var body openapi.ImportRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{
		"media_id": body.MediaId.String(),
		"kind":     string(body.Kind),
		"hub_id":   body.HubId.String(),
	}
	if body.Mapping != nil {
		mapping := map[string]any{}
		for key, value := range *body.Mapping {
			mapping[key] = value
		}
		in["mapping"] = mapping
	}
	out, ok := c.read(w, r, importEntriesUseCase, in)
	if !ok {
		return
	}
	resultURL := out.String("result_url")
	writeJSON(w, r, http.StatusAccepted, openapi.JobRef{
		JobId:     uuidValue(out.String("job_id")),
		Status:    openapi.JobStatusQUEUED,
		ResultUrl: &resultURL,
	})
}

// GetImport answers GET /imports/{importId}.
func (c *RestController) GetImport(w http.ResponseWriter, r *http.Request, importID openapi.ImportId) {
	out, ok := c.read(w, r, getImportUseCase, usecase.Input{"import_id": importID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, importRunResponse(out))
}

func importRunResponse(out usecase.Output) openapi.ImportRun {
	run := openapi.ImportRun{
		Id:        uuidValue(out.String("id")),
		Kind:      openapi.ImportKind(out.String("kind")),
		HubId:     uuidValue(out.String("hub_id")),
		Status:    openapi.ImportRunStatus(out.String("status")),
		CreatedAt: timeValue(out["created_at"]),
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
