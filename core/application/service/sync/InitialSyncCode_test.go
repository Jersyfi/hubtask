// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) CodeLinks(_ context.Context, after shared.ID, batch int) ([]repository.CodeLinkOf, error) {
	return pageAfter(s.codeLinks, func(r repository.CodeLinkOf) string { return r.Link.ID.String() }, after.String(), batch), nil
}

// UC-INT-13 check 6: an entry's code travels under the entry's collection to a device that may read
// it, and never to one that may not.
func TestTheWalkCarriesAnEntrysCodeWhereTheCollectionIsRead(t *testing.T) {
	pull, f, store := walking(t)
	seen, hidden := idAt('a', 1), idAt('a', 2)
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	store.codeLinks = []repository.CodeLinkOf{
		{Link: forge.Link{ID: idAt('f', 1), ItemID: seen, Kind: forge.PullRequest, Repository: "team/web", Ref: "7",
			State: forge.Merged, HappenedAt: at, UpdatedAt: at}, CollectionID: collectionA},
		{Link: forge.Link{ID: idAt('f', 2), ItemID: hidden, Kind: forge.Commit, Repository: "team/web", Ref: "abc",
			State: forge.Pushed, HappenedAt: at, UpdatedAt: at}, CollectionID: collectionB},
	}
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)
	var got []Record
	for _, record := range records {
		if record.Entity == "code_link" {
			got = append(got, record)
		}
	}
	if len(got) != 1 || got[0].ContainerID != collectionA || got[0].Payload["item_id"] != seen.String() ||
		got[0].Payload["state"] != "MERGED" {
		t.Errorf("code links delivered %v, want the readable entry's", got)
	}
}
