// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// An entry's kudos: read, given and taken back on one path, each answering the reader's own view.
// No ETag - kudos touch nothing of the entry's row, so there is no version to write against.

const (
	readKudosUseCase     = "ReadKudos"
	giveKudosUseCase     = "GiveKudos"
	takeBackKudosUseCase = "TakeBackKudos"
)

// ReadKudos answers GET /items/{itemId}/kudos.
func (c *RestController) ReadKudos(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	c.kudos(w, r, readKudosUseCase, itemID)
}

// GiveKudos answers PUT /items/{itemId}/kudos.
func (c *RestController) GiveKudos(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	c.kudos(w, r, giveKudosUseCase, itemID)
}

// TakeBackKudos answers DELETE /items/{itemId}/kudos.
func (c *RestController) TakeBackKudos(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	c.kudos(w, r, takeBackKudosUseCase, itemID)
}

func (c *RestController) kudos(w http.ResponseWriter, r *http.Request, name string, itemID openapi.ItemId) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())
	out, err := c.UseCases.Invoke(r.Context(), name, actor, usecase.Input{"item_id": itemID.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, kudosResponse(out))
}

// kudosResponse maps the catalogue's answer onto the contract's Kudos; `from` is empty rather than
// null for everybody the kudos were not given to.
func kudosResponse(out usecase.Output) openapi.Kudos {
	given, _ := out["given"].(bool)
	answer := openapi.Kudos{ItemId: uuidValue(out.String("item_id")), Given: given, From: []openapi.KudosFrom{}}
	from, _ := out["from"].([]any)
	for _, entry := range from {
		kudos, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		account, _ := kudos["account_id"].(string)
		answer.From = append(answer.From, openapi.KudosFrom{
			AccountId: uuidValue(account), GivenAt: timeValue(kudos["given_at"]),
		})
	}
	return answer
}
