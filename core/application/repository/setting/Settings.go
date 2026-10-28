// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package setting is the port of the settings catalogue's one storage shape: the entries a level
// stores, read for the places a resolution walks through and written one key at a time.
package setting

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Place is where an entry is stored: the workspace (ScopeID is the tenant), a hub, a collection
// or a person.
type Place struct {
	Level   domain.Level
	ScopeID shared.ID
}

// Stored is one entry as it is stored.
type Stored struct {
	Place     Place
	Key       domain.Key
	Entry     domain.Entry
	UpdatedAt time.Time
}

// Settings reads and writes the entries, inside the transaction's tenant.
type Settings interface {
	// Above answers the installation's and the transaction's workspace's plan's entries under a
	// key prefix, as the two layers above the workspace.
	Above(ctx context.Context, prefix string) (instance, plan domain.Layer, err error)
	// At answers the entries under a key prefix stored at the places.
	At(ctx context.Context, places []Place, prefix string) ([]Stored, error)
	// Put stores one entry, replacing what the place stored under the key; id is used for a new
	// row.
	Put(ctx context.Context, id shared.ID, place Place, key domain.Key, entry domain.Entry, at time.Time) error
	// Remove leaves the key to the level above, and reports whether the place stored it.
	Remove(ctx context.Context, place Place, key domain.Key) (bool, error)
}

// LayersOf groups stored entries into one layer per place, in the order of the places.
func LayersOf(places []Place, stored []Stored) []domain.Layer {
	layers := make([]domain.Layer, 0, len(places))
	for _, place := range places {
		layer := domain.Layer{Level: place.Level, Entries: map[domain.Key]domain.Entry{}}
		for _, entry := range stored {
			if entry.Place == place {
				layer.Entries[entry.Key] = entry.Entry
			}
		}
		layers = append(layers, layer)
	}
	return layers
}
