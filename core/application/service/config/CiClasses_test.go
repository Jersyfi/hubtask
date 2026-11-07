// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"
	"time"

	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	cmdbmodel "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A template's own class reads back from ListCiClasses as the block it was written from, so a
// workspace that took it shows no drift; the shipped classes are no blocks, and removing a block
// archives its class rather than deleting it.
func TestAWorkspacesOwnClassIsItsDocumentsBlock(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var blocks domain.Section
	for _, template := range templates {
		if template.Key == "configuration_items" {
			blocks = template.Document[sectionCiClasses]
		}
	}
	if len(blocks) == 0 {
		t.Fatal("the Configuration items template brings no class")
	}
	id, tenant := shared.ID("01936f2a-7c1e-7000-8000-000000000002"), shared.ID("01936f2a-7c1e-7000-8000-000000000001")
	var own []cmdbmodel.Class
	for key, block := range blocks {
		ops, _ := ciClassOps(domain.BlockChange{Section: sectionCiClasses, Key: key, Op: domain.OpAdd, After: block}, nil)
		class, err := cmdbmodel.NewClass(ciClassInput(ops[0].Input, id, tenant), cmdbmodel.NewClasses(nil))
		if err != nil {
			t.Fatal(err)
		}
		own = append(own, class)
	}
	classes := cmdbmodel.NewClasses(own)
	read := 0
	for _, class := range classes.All() {
		normal, err := normalise(map[string]any(cmdbservice.ClassOutput(class, classes.Resolve(class.Key))))
		if err != nil {
			t.Fatal(err)
		}
		key, block := ciClassBlock(normal.(map[string]any))
		if class.System {
			if key != "" {
				t.Errorf("the shipped class %s is a block", class.Key)
			}
			continue
		}
		read++
		if domain.Canonical(block) != domain.Canonical(blocks[key]) {
			t.Errorf("%s reads back as %v, written as %v", key, block, blocks[key])
		}
	}
	if read != len(blocks) {
		t.Errorf("%d of %d classes read back", read, len(blocks))
	}

	archived := time.Now()
	gone := own[0]
	gone.ArchivedAt = &archived
	normal, _ := normalise(map[string]any(cmdbservice.ClassOutput(gone, classes.Resolve(gone.Key))))
	if key, _ := ciClassBlock(normal.(map[string]any)); key != "" {
		t.Errorf("an archived class is a block: %s", key)
	}
	ops, err := ciClassOps(domain.BlockChange{Section: sectionCiClasses, Key: gone.Key, Op: domain.OpRemove, Before: blocks[gone.Key]}, nil)
	if err != nil || len(ops) != 1 || ops[0].UseCase != "DefineCiClass" || ops[0].Input["archived"] != true ||
		ops[0].Input["name"] != blocks[gone.Key]["name"] {
		t.Errorf("a removal archives the class as it stood: %+v, %v", ops, err)
	}
}
