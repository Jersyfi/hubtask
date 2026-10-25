// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Place is where a spoken entry goes: a collection, and - for a category of a simple list, such
// as "Metro" in "Einkauf" - the label it is filed under.
type Place struct {
	CollectionID shared.ID
	LabelID      shared.ID
	// Name is what Alexa says back: "Einkauf", or "Einkauf, Metro".
	Name string
}

// Known is one name a place is called by: its own name, a category's, or an alias somebody set.
type Known struct {
	Place Place
	Name  string
	// IsAlias marks a name somebody chose for speaking. It wins over a collection's own name, so
	// "Einkaufsliste" can be made to mean one list when several have similar names.
	IsAlias bool
	// Category marks a label's name. A collection's own name wins over a category of the same
	// name elsewhere.
	Category bool
}

// Resolution is what a spoken place came to.
type Resolution struct {
	Place Place
	// Found is false where nothing is called that; Ambiguous lists the candidates where several
	// are, so the assistant can ask which one.
	Found     bool
	Ambiguous []Place
}

// Resolve finds the place a spoken target names among the known ones.
//
// In order: an alias said exactly; a name said exactly; either without its list word ("die
// Einkaufsliste" names "Einkauf"); a name that starts with what was said. Several equally good
// answers are ambiguous; an alias among them decides.
func Resolve(target string, language Language, known []Known) Resolution {
	if strings.TrimSpace(target) == "" {
		return Resolution{}
	}
	said := Fold(target)
	bare := TargetWithout(target, language)
	tries := []func(Known) bool{
		func(each Known) bool { return each.IsAlias && Fold(each.Name) == said },
		func(each Known) bool { return Fold(each.Name) == said },
		func(each Known) bool { return each.IsAlias && Fold(each.Name) == bare },
		func(each Known) bool { return Fold(each.Name) == bare || TargetWithout(each.Name, language) == bare },
		func(each Known) bool {
			return bare != "" && strings.HasPrefix(Fold(each.Name), bare)
		},
	}
	for _, try := range tries {
		var matched []Place
		seen := map[Place]bool{}
		for _, each := range known {
			if try(each) && !seen[each.Place] {
				seen[each.Place] = true
				matched = append(matched, each.Place)
			}
		}
		// A collection's own name before a category called the same.
		if len(matched) > 1 {
			var own []Place
			for _, each := range known {
				if try(each) && !each.Category && !contains(own, each.Place) {
					own = append(own, each.Place)
				}
			}
			if len(own) > 0 {
				matched = own
			}
		}
		switch {
		case len(matched) == 1:
			return Resolution{Place: matched[0], Found: true}
		case len(matched) > 1:
			return Resolution{Ambiguous: matched}
		}
	}
	return Resolution{}
}

func contains(places []Place, place Place) bool {
	for _, each := range places {
		if each == place {
			return true
		}
	}
	return false
}
