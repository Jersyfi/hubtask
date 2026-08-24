// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The bulk route (C-11). Nothing here decides anything about an operation: which operations exist,
// what each one may do and what happens when one of them fails are the application layer's, once,
// whichever channel the call came through (ADR-0005). What this file owns is the one thing that is
// genuinely HTTP - the status each operation would have answered on its own.

const bulkUpdateWorkItemsUseCase = "BulkUpdateWorkItems"

// BulkUpdateWorkItems answers POST /items:bulk.
//
// HTTP 200 whenever the bulk was carried out, whatever happened inside it: a bulk that half
// succeeded is not a failed request, and a client reads what happened per operation rather than
// from one status covering five hundred of them (api-guidelines.md §5).
func (c *RestController) BulkUpdateWorkItems(
	w http.ResponseWriter, r *http.Request, _ openapi.BulkUpdateWorkItemsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.BulkUpdateWorkItemsJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"operations": bulkOperationsInput(body.Operations)}
	if body.Atomic != nil {
		in["atomic"] = *body.Atomic
	}

	out, err := c.UseCases.Invoke(r.Context(), bulkUpdateWorkItemsUseCase, actorOf(r), in)
	if err != nil {
		// The bulk itself could not be carried out - too many operations, an operation that is not
		// an object at all. An operation that merely failed never comes back this way.
		WriteProblem(w, err, requestID)
		return
	}

	writeJSON(w, r, http.StatusOK, struct {
		Results []bulkResult `json:"results"`
		Applied int          `json:"applied"`
		Failed  int          `json:"failed"`
	}{
		Results: bulkResultsResponse(out, requestID),
		Applied: out.Int("applied"),
		Failed:  out.Int("failed"),
	})
}

// bulkOperationsInput passes the operations on as the catalogue's untyped list.
//
// The payloads travel as they arrived: what a field means is the operation's own declaration, and
// an adapter that filtered them would be a second place deciding what an operation accepts.
func bulkOperationsInput(operations []openapi.BulkOperation) []any {
	sent := make([]any, 0, len(operations))

	for _, operation := range operations {
		entry := map[string]any{"op": string(operation.Op)}
		if operation.ItemId != nil {
			entry["item_id"] = operation.ItemId.String()
		}
		if operation.Payload != nil {
			entry["payload"] = map[string]any(*operation.Payload)
		}
		sent = append(sent, entry)
	}
	return sent
}

// bulkResult is the contract's BulkResult, with this package's problem in it rather than the
// generated one.
//
// The two serialise to the same schema - they are both api-guidelines.md §6's document - and the
// difference is that this one is what ProblemFrom produces, so a refusal inside a bulk is rendered
// by the same code that renders every refusal outside one. A second renderer for the same document
// is a second place for it to drift.
type bulkResult struct {
	Index   int               `json:"index"`
	Status  int               `json:"status"`
	Item    *openapi.WorkItem `json:"item,omitempty"`
	Problem *Problem          `json:"problem,omitempty"`
}

// bulkResultsResponse maps the catalogue's answer onto the contract's BulkResult, and gives each
// operation the status it would have answered on its own.
//
// The status is worked out here rather than carried in the answer, because a status is HTTP and the
// application layer speaks none: what it hands over is the refusal's own category, which is what
// api-guidelines.md §6 maps to a status - through the same table every other refusal goes through.
func bulkResultsResponse(out usecase.Output, requestID string) []bulkResult {
	results := []bulkResult{}

	entries, ok := out["results"].([]usecase.Output)
	if !ok {
		return results
	}
	for _, entry := range entries {
		result := bulkResult{Index: entry.Int("index")}

		if problem, refused := entry["problem"].(usecase.Output); refused {
			rendered := ProblemFrom(bulkRefusal(problem), requestID)
			result.Status, result.Problem = rendered.Status, &rendered
			results = append(results, result)
			continue
		}

		result.Status = bulkAppliedStatus(entry.String("op"))
		if item, carried := bulkItemOf(entry); carried {
			result.Item = &item
		}
		results = append(results, result)
	}
	return results
}

// bulkRefusal rebuilds the domain error the operation was refused with, so that the problem this
// answer carries is the one the single-entry route would have answered with - the same status, the
// same code, the same field findings.
func bulkRefusal(problem usecase.Output) error {
	refusal := shared.Error{
		Category:   shared.Category(problem.String("category")),
		Code:       problem.String("code"),
		DetailCode: problem.String("detail_code"),
	}
	if params, held := problem["params"].(map[string]string); held {
		refusal.Params = params
	}
	if findings, held := problem["field_errors"].([]usecase.Output); held {
		for _, finding := range findings {
			field := shared.FieldError{Path: finding.String("path"), Code: finding.String("code")}
			if params, carried := finding["params"].(map[string]string); carried {
				field.Params = params
			}
			refusal.Fields = append(refusal.Fields, field)
		}
	}
	return &refusal
}

// bulkAppliedStatus is the status the operation would have answered on its own: a creation answers
// 201 and everything else answers 200, exactly as the routes of the same names do.
func bulkAppliedStatus(op string) int {
	if op == "CREATE_ITEM" {
		return http.StatusCreated
	}
	return http.StatusOK
}

// bulkItemOf reads the entry out of whatever the operation answered.
//
// The operations answer different shapes - a move answers its result and its losses, a completion
// answers the entry itself - and the contract's BulkResult carries the entry. So the entry is taken
// from the `item` an answer wraps it in, or from the answer itself when it is one; an operation
// whose answer holds no entry carries none, which is what the schema's optional field is for.
func bulkItemOf(entry usecase.Output) (openapi.WorkItem, bool) {
	out, held := entry["output"].(usecase.Output)
	if !held {
		return openapi.WorkItem{}, false
	}
	if nested, wrapped := out["item"].(usecase.Output); wrapped {
		return workItemResponse(nested), true
	}
	if out.String("id") == "" {
		return openapi.WorkItem{}, false
	}
	return workItemResponse(out), true
}
