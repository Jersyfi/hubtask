// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// An entry's kudos are read, given and taken back by the generated mapping (Doors.gen.go), each
// answering the reader's own view. No ETag - kudos touch nothing of the entry's row.

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
