// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
)

// sectionTimeWorkTypes is the workspace's kinds of work time is recorded as (configuration.md §6).
// An archived kind is no block, and removing a block archives its kind: records keep it.
const sectionTimeWorkTypes = "time_work_types"

// workTypeAttributes is what a block keeps of a kind of work, each as DefineWorkType takes it.
var workTypeAttributes = []string{"name", "billable", "position"}

func timeWorkTypesSection() section {
	return section{
		name: sectionTimeWorkTypes, module: module.Time, list: "ListWorkTypes", rows: "data", key: "key",
		attributes: workTypeAttributes,
		// DefineWorkType both makes and changes a kind; ops writes it whole.
		create: "DefineWorkType", change: "DefineWorkType",
		block: workTypeBlock, ops: workTypeOps,
	}
}

// workTypeBlock reads a kind in the document's shape; an archived one is none, and a kind that
// starts unbillable says nothing of it, so a template's block reads back as it was written.
func workTypeBlock(row map[string]any) (string, domain.Block) {
	if archived, _ := row["archived"].(bool); archived {
		return "", nil
	}
	block := domain.Block{"name": row["name"], "position": number(row["position"])}
	if billable, _ := row["billable"].(bool); billable {
		block["billable"] = true
	}
	return text(row["key"]), block
}

// workTypeOps defines the kind whole from its block; a removal archives it as it stood.
func workTypeOps(change domain.BlockChange, _ map[string]any) ([]operation, error) {
	block, archived := change.After, false
	if change.Op == domain.OpRemove {
		block, archived = change.Before, true
	}
	in := usecase.Input{"key": change.Key, "archived": archived}
	for _, attribute := range workTypeAttributes {
		if value, held := block[attribute]; held && value != nil {
			in[attribute] = value
		}
	}
	return []operation{{UseCase: "DefineWorkType", Path: change.Path(), Input: in}}, nil
}
