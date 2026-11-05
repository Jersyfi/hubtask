// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// hubCollections answers the stored collections of each hub that are neither trashed nor archived,
// as the store's statement does, up to perHub of each.
type hubCollections struct {
	store *containers
	extra int
	err   error
}

func (h hubCollections) CollectionsOfHubs(
	_ context.Context, hubIDs []shared.ID, perHub int,
) (map[shared.ID][]domain.Container, error) {
	if h.err != nil {
		return nil, h.err
	}
	found := map[shared.ID][]domain.Container{}
	for _, hub := range hubIDs {
		for _, c := range h.store.stored {
			if c.ParentID == hub && c.Type == domain.ContainerCollection && c.DeletedAt == nil && c.ArchivedAt == nil &&
				len(found[hub]) < perHub {
				found[hub] = append(found[hub], c)
			}
		}
		for i := 0; i < h.extra && len(found[hub]) < perHub; i++ {
			found[hub] = append(found[hub], countedCollection(shared.ID(fmt.Sprintf("0192f000-0000-7000-8000-%012d", i))))
		}
	}
	return found, nil
}

func readHub(t *testing.T, h *structureHarness, store *figureStore, collections hubCollections) map[string]any {
	t.Helper()
	figures := FigureReader{Figures: store, Clock: clock.Fixed(now), Profiles: &profiles{rows: seededProfiles()}}
	get := GetContainer{
		Containers: h.containers, Authorizer: h.authorizer, UnitOfWork: h.uow,
		Structure: h.set.Structure, Figures: figures,
		HubFigures: HubFigureReader{Collections: collections, Structure: h.set.Structure, Figures: figures},
	}
	registry, err := usecase.NewRegistry(nil, get.Descriptor())
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	out, err := registry.Invoke(context.Background(), GetContainerName, actor(), usecase.Input{"container_id": hubID.String()})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	figure, _ := out["figure"].(map[string]any)
	return figure
}

// A hub's read answers its figure (user-experience.md §2.5): the open entries of the collections
// the tree shows under it, an archived one left out; unavailable rather than smaller where one
// could not be counted or where it holds more than one read counts.
func TestAHubReadAnswersTheOpenEntriesOfItsCollections(t *testing.T) {
	h := newStructureHarness(t)
	archived := h.containers.stored[otherCollectionID]
	archivedAt := now
	archived.ArchivedAt = &archivedAt
	h.containers.stored["0192f000-0000-7000-8000-0000000000e1"] = domain.Container{
		ID: "0192f000-0000-7000-8000-0000000000e1", TenantID: tenantID, Type: domain.ContainerCollection,
		ParentID: hubID, Name: "Paint", OrderKey: "a2", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[otherCollectionID] = archived
	counts := map[shared.ID]domain.FigureCounts{
		collectionID: {Open: 4}, "0192f000-0000-7000-8000-0000000000e1": {Open: 3}, otherCollectionID: {Open: 50},
	}

	store := &figureStore{counts: counts}
	figure := readHub(t, h, store, hubCollections{store: h.containers})
	if figure["kind"] != "OPEN" || figure["open"] != 7 || figure["unavailable"] != nil {
		t.Fatalf("the hub's figure is %v, want 7 open", figure)
	}
	for _, asked := range store.asked {
		if asked.Kind != domain.FigureOpen || asked.CollectionID == otherCollectionID {
			t.Errorf("counted %+v", asked)
		}
	}

	if figure := readHub(t, h, &figureStore{err: errors.New("statement timeout")}, hubCollections{store: h.containers}); figure["unavailable"] != true || figure["open"] != nil {
		t.Errorf("a failed count answered %v", figure)
	}
	if figure := readHub(t, h, &figureStore{counts: counts}, hubCollections{err: errors.New("down")}); figure["unavailable"] != true {
		t.Errorf("an unread hub answered %v", figure)
	}
	crowded := readHub(t, h, &figureStore{counts: counts}, hubCollections{store: h.containers, extra: MaxHubFigureCollections})
	if crowded["unavailable"] != true {
		t.Errorf("a hub of more than %d collections answered %v", MaxHubFigureCollections, crowded)
	}
}
