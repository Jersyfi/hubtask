// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// The one trash (user-experience.md §5.9): every kind the contract restores out of a trash is a
// kind /trash lists, so a new trashable kind cannot keep a trash list of its own.
func TestEveryKindWithARestoreIsInTheOneTrash(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	var entry struct {
		Properties struct {
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(contract.Components.Schemas["TrashEntry"], &entry); err != nil {
		t.Fatal(err)
	}
	listed := entry.Properties.Kind.Enum

	// The resource a `:restore` is on, and the trash kind it restores.
	kinds := map[string]string{
		"containers": "CONTAINER", "items": "ITEM", "pages": "PAGE", "whiteboards": "WHITEBOARD",
	}
	// Restores that take nothing out of a trash.
	notTrash := map[string]string{
		"labels":   "a deleted label stays in its collection's vocabulary and has no trash period",
		"versions": "restoring a version writes a new one; nothing was deleted",
	}

	restores := 0
	for path := range contract.Paths {
		if !strings.HasSuffix(path, ":restore") {
			continue
		}
		restores++
		resource := ""
		for _, segment := range strings.Split(strings.TrimSuffix(path, ":restore"), "/") {
			if segment != "" && !strings.HasPrefix(segment, "{") {
				resource = segment
			}
		}
		if _, exempt := notTrash[resource]; exempt {
			continue
		}
		kind, known := kinds[resource]
		if !known {
			t.Errorf("%s restores %q, which /trash does not list: add the kind to TrashEntry and ListTrash", path, resource)
			continue
		}
		if !slices.Contains(listed, kind) {
			t.Errorf("%s restores a %s, but TrashEntry.kind lists only %v", path, kind, listed)
		}
	}
	if restores == 0 {
		t.Fatal("the contract has no :restore operation; the test reads the wrong file")
	}
}
