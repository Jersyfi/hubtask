// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) FileLinks(_ context.Context, after shared.ID, batch int) ([]repository.FileLinkOf, error) {
	return pageAfter(s.fileLinks, func(r repository.FileLinkOf) string { return r.Link.ID.String() }, after.String(), batch), nil
}

// UC-INT-14 check 5: an entry's file links travel under the entry's collection to a device that
// may read it, and never to one that may not.
func TestTheWalkCarriesAnEntrysFilesWhereTheCollectionIsRead(t *testing.T) {
	pull, f, store := walking(t)
	seen, hidden := idAt('a', 1), idAt('a', 2)
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	store.fileLinks = []repository.FileLinkOf{
		{Link: filelink.Link{ID: idAt('e', 1), ItemID: seen, Provider: filelink.Nextcloud,
			Address: "https://cloud.example.org/s/abc", Name: "Spec.pdf", AddedAt: at}, CollectionID: collectionA},
		{Link: filelink.Link{ID: idAt('e', 2), ItemID: hidden, Provider: filelink.SharePoint,
			Address: "https://contoso.sharepoint.com/a.docx", Name: "a.docx", AddedAt: at}, CollectionID: collectionB},
	}
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)
	var got []Record
	for _, record := range records {
		if record.Entity == "file_link" {
			got = append(got, record)
		}
	}
	if len(got) != 1 || got[0].ContainerID != collectionA || got[0].Payload["item_id"] != seen.String() ||
		got[0].Payload["provider"] != "NEXTCLOUD" {
		t.Errorf("file links delivered %v, want the readable entry's", got)
	}
}
