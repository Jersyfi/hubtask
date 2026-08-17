// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	tenant   = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	hubID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	newID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	actorID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	created  = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	baseHub  = work.NewContainerInput{ID: newID, TenantID: tenant, Type: work.ContainerHub, Name: "Private", OrderKey: "m", CreatedBy: actorID, Now: created}
	baseColl = work.NewContainerInput{ID: newID, TenantID: tenant, Type: work.ContainerCollection, ParentID: hubID, Name: "Shopping", OrderKey: "m", CreatedBy: actorID, Now: created}
)

func TestNewContainerAcceptsBothLevels(t *testing.T) {
	t.Run("a hub has no parent", func(t *testing.T) {
		container, err := work.NewContainer(baseHub)
		if err != nil {
			t.Fatalf("a hub without a parent was refused: %v", err)
		}
		if !container.ParentID.IsZero() || container.Version != 1 {
			t.Errorf("unexpected container: %+v", container)
		}
		if container.CreatedAt != created || container.UpdatedAt != created {
			t.Errorf("the timestamps come from the clock port, not from the constructor: %+v", container)
		}
	})

	t.Run("a collection sits in its hub", func(t *testing.T) {
		container, err := work.NewContainer(baseColl)
		if err != nil {
			t.Fatalf("a collection under a hub was refused: %v", err)
		}
		if container.ParentID != hubID {
			t.Errorf("the parent was not kept: %+v", container)
		}
	})
}

// The name is what a person compares, so the constructor compares the same thing: trimmed, in code
// points, and on one line.
func TestNewContainerChecksTheName(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantName   string
		detailCode string
	}{
		{name: "trimmed", input: "  Team  ", wantName: "Team"},
		{name: "combining marks count as one code point each", input: strings.Repeat("é", 100), wantName: strings.Repeat("é", 100)},
		{name: "at the limit", input: strings.Repeat("a", 200), wantName: strings.Repeat("a", 200)},
		{name: "empty", input: "", detailCode: "containers.name_empty"},
		{name: "whitespace only", input: " \t ", detailCode: "containers.name_empty"},
		{name: "one code point too long", input: strings.Repeat("a", 201), detailCode: "containers.name_too_long"},
		{name: "a newline", input: "Team\nB", detailCode: "containers.name_malformed"},
		{name: "a C1 control", input: "Team\u0085B", detailCode: "containers.name_malformed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := baseHub
			in.Name = c.input

			container, err := work.NewContainer(in)
			if c.detailCode == "" {
				if err != nil {
					t.Fatalf("%q was refused: %v", c.input, err)
				}
				if container.Name != c.wantName {
					t.Errorf("name %q, want %q", container.Name, c.wantName)
				}
				return
			}

			assertDetail(t, err, shared.ErrValidation, c.detailCode)
			if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != "/name" {
				t.Errorf("the finding does not point at the field: %+v", fields)
			}
		})
	}
}

// I-C1 in both directions.
func TestNewContainerChecksTheParentInvariant(t *testing.T) {
	t.Run("a hub with a parent", func(t *testing.T) {
		in := baseHub
		in.ParentID = hubID

		_, err := work.NewContainer(in)
		assertDetail(t, err, shared.ErrValidation, "containers.hub_has_no_parent")
	})

	t.Run("a collection without a parent", func(t *testing.T) {
		in := baseColl
		in.ParentID = ""

		_, err := work.NewContainer(in)
		assertDetail(t, err, shared.ErrValidation, "containers.collection_needs_parent")
	})
}

func TestNewContainerRefusesAnUnknownType(t *testing.T) {
	in := baseHub
	in.Type = "PROJECT"

	_, err := work.NewContainer(in)
	assertDetail(t, err, shared.ErrValidation, "containers.type_unknown")
}

// What comes from a port rather than from a client fails as a defect, not as a validation error:
// nothing the caller sends could have prevented it.
func TestNewContainerRefusesAnIncompleteIdentity(t *testing.T) {
	for _, missing := range []string{"id", "tenant", "actor", "order key"} {
		t.Run("without an "+missing, func(t *testing.T) {
			in := baseHub
			switch missing {
			case "id":
				in.ID = ""
			case "tenant":
				in.TenantID = ""
			case "actor":
				in.CreatedBy = ""
			case "order key":
				in.OrderKey = ""
			}

			_, err := work.NewContainer(in)
			assertDetail(t, err, shared.ErrInternal, "containers.identity_incomplete")
		})
	}
}

func TestContainerTypeHierarchy(t *testing.T) {
	cases := []struct {
		parent, child work.ContainerType
		want          bool
	}{
		{work.ContainerHub, work.ContainerCollection, true},
		{work.ContainerHub, work.ContainerHub, false},
		{work.ContainerCollection, work.ContainerCollection, false},
		{work.ContainerCollection, work.ContainerHub, false},
	}

	for _, c := range cases {
		if got := c.parent.AllowsChild(c.child); got != c.want {
			t.Errorf("%s under %s: %v, want %v", c.child, c.parent, got, c.want)
		}
	}

	if len(work.ContainerTypes()) != 2 {
		t.Errorf("the set of types changed without this test: %v", work.ContainerTypes())
	}
	if work.ContainerType("HUB").Valid() == work.ContainerType("hub").Valid() {
		t.Error("the type is compared case-sensitively, as the enum in the contract is")
	}
}

// I-C3: an archived container is read-only, and what would be created inside it inherits that.
func TestEnsureAcceptsChildren(t *testing.T) {
	archived, trashed := created, created

	cases := []struct {
		name       string
		parent     work.Container
		child      work.ContainerType
		detailCode string
		category   *shared.Error
	}{
		{
			name:   "a collection in an active hub",
			parent: work.Container{ID: hubID, Type: work.ContainerHub},
			child:  work.ContainerCollection,
		},
		{
			name:       "a collection in an archived hub",
			parent:     work.Container{ID: hubID, Type: work.ContainerHub, ArchivedAt: &archived},
			child:      work.ContainerCollection,
			detailCode: "containers.parent_archived",
			category:   shared.ErrConflict,
		},
		{
			name:       "a collection in a trashed hub",
			parent:     work.Container{ID: hubID, Type: work.ContainerHub, DeletedAt: &trashed},
			child:      work.ContainerCollection,
			detailCode: "containers.parent_trashed",
			category:   shared.ErrConflict,
		},
		{
			name:       "a hub in a hub",
			parent:     work.Container{ID: hubID, Type: work.ContainerHub},
			child:      work.ContainerHub,
			detailCode: "containers.parent_type_invalid",
			category:   shared.ErrValidation,
		},
		{
			name:       "a collection in a collection",
			parent:     work.Container{ID: hubID, Type: work.ContainerCollection},
			child:      work.ContainerCollection,
			detailCode: "containers.parent_type_invalid",
			category:   shared.ErrValidation,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.parent.EnsureAcceptsChildren(c.child)
			if c.detailCode == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			assertDetail(t, err, c.category, c.detailCode)
		})
	}
}

// Trashed beats archived: a container in the trash is gone as far as a client is concerned, and
// reporting it as merely archived would invite a retry that cannot succeed.
func TestTrashedIsReportedBeforeArchived(t *testing.T) {
	at := created
	parent := work.Container{ID: hubID, Type: work.ContainerHub, ArchivedAt: &at, DeletedAt: &at}

	if !parent.IsArchived() || !parent.IsTrashed() {
		t.Fatal("the lifecycle predicates disagree with the timestamps")
	}
	assertDetail(t, parent.EnsureAcceptsChildren(work.ContainerCollection),
		shared.ErrConflict, "containers.parent_trashed")
}

func assertDetail(t *testing.T, err error, sentinel *shared.Error, detailCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %s", detailCode)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("category %s, want %s", shared.AsError(err).Category, sentinel.Category)
	}
	if got := shared.AsError(err).DetailCode; got != detailCode {
		t.Errorf("detail code %s, want %s", got, detailCode)
	}
}
