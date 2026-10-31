// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The read side of the containers and the items. Four handlers, no rules: each reads the
// request, calls one catalogue entry, and maps the result - the permission check, the projection and
// the paging all happen in the application layer, once, for every channel (ADR-0005, arc42 §4).

const (
	getContainerUseCase = "GetContainer"
)

// expansionsOf reads the relations a client asked for and reports which of them this installation
// serves.
//
// `labels` is served. Everything else the contract's example names - children, the
// assignee, the cover - is not, and is refused by name rather
// than ignored: a client that asked for children and received an entry without them cannot tell
// that from an entry that has none, and would render an empty tree (api-guidelines.md §6).
//
// The list is comma separated in one query parameter (`style: form, explode: false`), and the
// generated type hands it over already split. An empty entry is skipped rather than refused:
// `?expand=` binds as one empty string rather than as no parameter, and a client that always sends
// the parameter and leaves it empty is asking for the plain entry.
func expansionsOf(expand *[]string) (labels bool, err error) {
	asked, err := expansionsAllowing(expand, false)
	return asked.labels, err
}

// expansions is what a read was asked to include besides the entries themselves.
type expansions struct {
	labels bool
	// counts is served by `POST /items:query` alone: what each entry holds, counted for the page.
	counts bool
}

// expansionsAllowing reads `expand`, refusing `counts` where the endpoint does not serve it.
func expansionsAllowing(expand *[]string, mayCount bool) (expansions, error) {
	var asked expansions
	if expand == nil {
		return asked, nil
	}

	for _, relation := range *expand {
		switch relation = strings.TrimSpace(relation); relation {
		case "":
			continue
		case "labels":
			asked.labels = true
		case "counts":
			if !mayCount {
				return expansions{}, expandNotSupported(relation)
			}
			asked.counts = true
		default:
			return expansions{}, expandNotSupported(relation)
		}
	}
	return asked, nil
}

func expandNotSupported(relation string) error {
	return shared.ErrValidation.
		WithDetail("items.expand_not_supported").
		WithParams(map[string]string{"value": relation}).
		WithFields(shared.FieldError{Path: "/expand", Code: "items.expand_not_supported"})
}

// read runs one catalogue entry and reports whether the caller may go on to write a body. The four
// handlers above share it because the three lines it holds - the wiring check, the actor, the problem
// document - are identical in all of them, and a fourth copy is a fourth place to forget one.
func (c *RestController) read(
	w http.ResponseWriter, r *http.Request, name string, in usecase.Input,
) (usecase.Output, bool) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return nil, false
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := c.UseCases.Invoke(r.Context(), name, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return nil, false
	}
	return out, true
}

// rowsOf reads the page's rows out of the catalogue's answer.
//
// Empty rather than nil when the shape is not what it should be. A missing `data` is a defect in a use
// case, and the handler's job at that point is to answer the shape the contract promises rather than to
// panic on a type assertion halfway through writing a body.
func rowsOf(out usecase.Output) []usecase.Output {
	rows, _ := out["data"].([]usecase.Output)
	return rows
}

// pageResponse maps the walk's state. next_cursor is a pointer with no omitempty in the generated
// type, so the last page carries an explicit null rather than no field at all - which is what lets a
// client read the field unconditionally.
func pageResponse(out usecase.Output) openapi.PageInfo {
	state, held := out["page"].(map[string]any)
	if !held {
		state, _ = out["page"].(usecase.Output)
	}
	hasMore, _ := state["has_more"].(bool)

	info := openapi.PageInfo{HasMore: hasMore}
	if cursor, ok := state["next_cursor"].(string); ok && cursor != "" {
		info.NextCursor = &cursor
	}
	return info
}
