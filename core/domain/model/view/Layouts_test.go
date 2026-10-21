// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A collection's views setting offers the layouts a saved view stores. The work package declares
// its own copy because this one depends on it; the two lists are one vocabulary.
func TestACollectionOffersTheLayoutsASavedViewStores(t *testing.T) {
	offered := work.ViewLayouts()
	if len(offered) != len(Layouts()) {
		t.Fatalf("the views setting knows %d layouts, a saved view %d", len(offered), len(Layouts()))
	}
	for i, layout := range Layouts() {
		if string(offered[i]) != string(layout) {
			t.Errorf("layout %d: %s against %s", i, offered[i], layout)
		}
	}
}
