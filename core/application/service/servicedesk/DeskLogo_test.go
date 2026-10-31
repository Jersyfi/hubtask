// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	logoID  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e101")
	otherID = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e102")
)

type logoStore struct{ logos map[shared.ID]shared.ID }

func (s *logoStore) Find(_ context.Context, collectionID shared.ID) (shared.ID, error) {
	return s.logos[collectionID], nil
}

func (s *logoStore) Set(_ context.Context, collectionID, mediaID shared.ID) error {
	s.logos[collectionID] = mediaID
	return nil
}

type logoObjects struct {
	mediarepo.Objects
	stored map[shared.ID]media.Object
	counts map[shared.ID]int
}

func (o *logoObjects) Find(_ context.Context, id shared.ID) (media.Object, error) {
	object, ok := o.stored[id]
	if !ok {
		return media.Object{}, shared.ErrNotFound
	}
	return object, nil
}

func (o *logoObjects) AdjustRefCount(_ context.Context, id shared.ID, delta int) error {
	o.counts[id] += delta
	return nil
}

func logoHarness(allow bool) (*logoStore, *logoObjects, *gate, Desk) {
	logos := &logoStore{logos: map[shared.ID]shared.ID{collection: otherID}}
	objects := &logoObjects{stored: map[shared.ID]media.Object{
		logoID: {ID: logoID, Usage: media.UsageDeskLogo, Status: media.StatusReady, ContentType: "image/png", CreatedBy: handlerID},
	}, counts: map[shared.ID]int{}}
	permission := &gate{allow: allow}
	desk := Desk{Containers: brandedDesk{}, Authorizer: permission, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{}}
	return logos, objects, permission, desk
}

// UC-SVC-05 check 9: whoever may change the collection's structure gives its desk a logo of their
// own upload; the logo it replaces is let go, and anybody else changes nothing.
func TestADeskShowsALogoItsConfigurerUploaded(t *testing.T) {
	logos, objects, permission, desk := logoHarness(true)
	out, err := SetDeskLogo{Desk: desk, Logos: logos, Objects: objects}.Descriptor().Handler.Invoke(
		context.Background(), reader(), usecase.Input{"container_id": collection.String(), "media_id": logoID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out["logo_media_id"] != logoID.String() || logos.logos[collection] != logoID {
		t.Fatalf("the desk shows %v, stored %v", out, logos.logos[collection])
	}
	if objects.counts[logoID] != 1 || objects.counts[otherID] != -1 {
		t.Errorf("the counts moved %v", objects.counts)
	}
	if len(permission.asked) != 1 || permission.asked[0].Permission != service.PermissionStructure {
		t.Errorf("asked %+v, want STRUCTURE at the collection", permission.asked)
	}

	out, err = RemoveDeskLogo{Desk: desk, Logos: logos, Objects: objects}.Descriptor().Handler.Invoke(
		context.Background(), reader(), usecase.Input{"container_id": collection.String()})
	if err != nil || out["logo_media_id"] != nil || !logos.logos[collection].IsZero() || objects.counts[logoID] != 0 {
		t.Errorf("removing answered %v, %v; stored %v, counts %v", out, err, logos.logos[collection], objects.counts)
	}

	refused, objects, _, desk := logoHarness(false)
	_, err = SetDeskLogo{Desk: desk, Logos: refused, Objects: objects}.Execute(context.Background(), reader(), collection, logoID)
	if err == nil || refused.logos[collection] != otherID {
		t.Errorf("a refused configurer set the logo: %v", err)
	}
}

// An object somebody else uploaded is answered as missing, and one that is not an image as such.
func TestADesksLogoIsOnlyAnImageOfOnesOwnUpload(t *testing.T) {
	logos, objects, _, desk := logoHarness(true)
	foreign := objects.stored[logoID]
	foreign.CreatedBy = otherID
	objects.stored[logoID] = foreign
	_, err := SetDeskLogo{Desk: desk, Logos: logos, Objects: objects}.Execute(context.Background(), reader(), collection, logoID)
	if shared.AsError(err).DetailCode != "media.not_found" {
		t.Errorf("somebody else's upload answered %v", err)
	}
	foreign.CreatedBy, foreign.ContentType = handlerID, "image/svg+xml"
	objects.stored[logoID] = foreign
	_, err = SetDeskLogo{Desk: desk, Logos: logos, Objects: objects}.Execute(context.Background(), reader(), collection, logoID)
	if shared.AsError(err).DetailCode != "media.logo_not_an_image" {
		t.Errorf("an SVG answered %v", err)
	}
}
