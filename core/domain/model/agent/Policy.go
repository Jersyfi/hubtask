// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import "github.com/Jersyfi/hubtask/core/domain/model/setting"

// RegisteredOnlyKey is the workspace's switch refusing an unregistered caller on MCP: a person's own
// token or a service account no agent names (UC-AGT-02). Off by default, since refusing them changes
// what an integrator observes; on, every act through MCP is a registered agent's, governed.
const RegisteredOnlyKey setting.Key = "agents.registered_only"

// RegisteredOnlyDefinition is its declaration: the installation and a plan may switch it on and lock
// it, and a workspace may only tighten what they leave - switch it on.
func RegisteredOnlyDefinition() setting.Definition {
	return setting.Definition{
		Key: RegisteredOnlyKey, Kind: setting.KindFlag,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayTighten, Default: false,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
		Tightens: func(inForce, proposed any) bool {
			on, _ := proposed.(bool)
			held, _ := inForce.(bool)
			return on || !held
		},
	}
}

// RegisteredOnlyOf reads the resolved switch; anything but true is off.
func RegisteredOnlyOf(r setting.Resolution) bool {
	on, _ := r.Effective.(bool)
	return on
}
