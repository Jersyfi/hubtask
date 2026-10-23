// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-24 check 6: a flatten moves exactly what the structure hides by level, deepest first, so
// each entry is a leaf when it moves.
func TestAFlattenMovesWhatIsHiddenDeepestFirst(t *testing.T) {
	task := shared.MustParseID("0192f000-0000-7000-8000-000000000401")
	pack := shared.MustParseID("0192f000-0000-7000-8000-000000000402")
	step := shared.MustParseID("0192f000-0000-7000-8000-000000000403")
	call := shared.MustParseID("0192f000-0000-7000-8000-000000000404")
	collection := shared.MustParseID("0192f000-0000-7000-8000-000000000405")
	packPath := domain.RootPath(task) + pack.String() + domain.PathSeparator
	shapes := []repository.EntryShape{
		{ID: task, CollectionID: collection, Type: domain.ItemTask, Path: domain.RootPath(task)},
		{ID: pack, CollectionID: collection, Type: domain.ItemWorkPackage, Path: packPath},
		{ID: step, CollectionID: collection, Type: domain.ItemActivity, Path: packPath + step.String() + domain.PathSeparator},
		{ID: call, CollectionID: collection, Type: domain.ItemActivity, Path: domain.RootPath(task) + call.String() + domain.PathSeparator},
	}
	bound := domain.NewStructureBound(matrixProfiles(), matrixProfiles())
	structure := func(levels ...domain.ItemType) domain.EffectiveStructure {
		values := domain.StructureValues{}
		for _, level := range levels {
			values.Levels = append(values.Levels, domain.Level{Type: level})
		}
		return domain.EffectiveStructure{Values: values}
	}
	ids := func(order []repository.EntryShape) []shared.ID {
		out := []shared.ID{}
		for _, shape := range order {
			out = append(out, shape.ID)
		}
		return out
	}

	for _, tc := range []struct {
		name   string
		levels []domain.ItemType
		want   []shared.ID
	}{
		{"every level offered moves nothing", []domain.ItemType{domain.ItemTask, domain.ItemWorkPackage, domain.ItemActivity}, []shared.ID{}},
		{"tasks alone move the deepest activity first, then the next level by path", []domain.ItemType{domain.ItemTask}, []shared.ID{step, pack, call}},
		{"tasks and activities move the work package's activity, then the work package", []domain.ItemType{domain.ItemTask, domain.ItemActivity}, []shared.ID{step, pack}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ids(flatteningOrder(domain.NewOffered(bound, structure(tc.levels...)), shapes))
			want := tc.want
			if !slices.Equal(got, want) {
				t.Errorf("moved %v, want %v", got, want)
			}
			count := flattenCount(bound, map[shared.ID]domain.EffectiveStructure{collection: structure(tc.levels...)}, shapes)
			if count != len(tc.want) {
				t.Errorf("the preview counts %d, the flatten moves %d", count, len(tc.want))
			}
		})
	}
}
