// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-WRK-39 check 5: an agent is offered the move pre-check as a tool that only reads, with the
// fields the move itself takes.
func TestTheMovePreviewIsAReadOnlyTool(t *testing.T) {
	tools := ToolsOf([]usecase.Descriptor{work.PreviewContainerMove{}.Descriptor()})
	if len(tools) != 1 {
		t.Fatalf("%d tools", len(tools))
	}
	tool := tools[0]
	if tool.Name == "" || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint {
		t.Errorf("the tool %q is not offered as reading only: %+v", tool.Name, tool.Annotations)
	}
	properties, _ := tool.InputSchema["properties"].(map[string]any)
	for _, field := range []string{"container_id", "target_parent_id", "before_container_id"} {
		if _, ok := properties[field]; !ok {
			t.Errorf("the tool does not take %s: %v", field, properties)
		}
	}
}
