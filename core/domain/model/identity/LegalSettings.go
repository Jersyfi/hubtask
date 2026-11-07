// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "github.com/Jersyfi/hubtask/core/domain/model/setting"

// The legal links as entries of the settings catalogue (domain-model.md §2, "The settings
// catalogue"). A lock pins its level's own link, "no link" included, so a locked layer stores its
// value even when it is empty; an open layer stores only a link it set.

// LegalArea is the area the legal links are keyed under.
const LegalArea = "legal"

// LegalKey is a link's catalogue key.
func LegalKey(link LegalLink) setting.Key { return setting.Key(LegalArea + "." + string(link)) }

// LegalDefinition is one link's catalogue entry: the installation and a plan set and lock it, a
// workspace replaces it; the provider's account link is the installation's and a plan's alone.
func LegalDefinition(link LegalLink) setting.Definition {
	declaration := DeclarationOfLink(link)
	levels := []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace}
	writers := map[setting.Level]string{
		setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
		setting.LevelWorkspace: "MANAGE_MEMBERS",
	}
	if link == LinkAccount {
		levels = levels[:2]
		delete(writers, setting.LevelWorkspace)
	}
	return setting.Definition{
		Key: LegalKey(link), Kind: setting.Kind(declaration.Kind),
		Levels: levels, Lockable: true, May: setting.May(declaration.May),
		Writers: writers, Default: "",
	}
}

// legalLayerAt is one level's links as catalogue entries; locks says whether its locks are read.
func legalLayerAt(level setting.Level, layer LegalLayer, locks bool, names ...LegalLink) setting.Layer {
	entries := map[setting.Key]setting.Entry{}
	for _, name := range names {
		value := layer.Links.Of(name)
		switch {
		case locks && layer.Locks[name]:
			entries[LegalKey(name)] = setting.Entry{Value: value, Hold: setting.HoldLock}
		case value != "":
			entries[LegalKey(name)] = setting.Entry{Value: value, Hold: setting.HoldSet}
		}
	}
	return setting.Layer{Level: level, Entries: entries}
}

// resolveLegal answers one link through the catalogue. A workspace's locks are never read: it has
// nobody below it to lock.
func resolveLegal(name LegalLink, instance, plan, workspace LegalLayer) setting.Resolution {
	return setting.Resolve(LegalDefinition(name),
		legalLayerAt(setting.LevelInstance, instance, true, name),
		legalLayerAt(setting.LevelPlan, plan, true, name),
		legalLayerAt(setting.LevelWorkspace, workspace, false, name),
	)
}
