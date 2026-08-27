// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event_test

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
)

// The map has to cover the closed set. A missing entry answers "", which is a scope no token can
// hold - so the event would be unpollable, and nothing but a subscriber would ever notice.
func TestEveryEventTypeHasAReadScope(t *testing.T) {
	for _, eventType := range event.Types() {
		if eventType.ReadScope() == "" {
			t.Errorf("%s has no read scope: add it to readScopes", eventType)
		}
	}
}

// Reading, never writing. Watching work being finished is not permission to finish it, and a
// polling endpoint that accepted a write scope would be a way around the read one.
func TestNoEventIsReadWithAWriteScope(t *testing.T) {
	for _, eventType := range event.Types() {
		if strings.HasSuffix(eventType.ReadScope(), ":write") {
			t.Errorf("%s is scoped %q, which is a write scope", eventType, eventType.ReadScope())
		}
	}
}

// An unknown type has no scope, and that is what makes a poll of one a refusal rather than an
// empty page: the use case checks Valid() first, and this is the second answer behind it.
func TestAnUnknownTypeHasNoReadScope(t *testing.T) {
	if scope := event.Type("de.hubtask.work.item.invented.v1").ReadScope(); scope != "" {
		t.Errorf("an undeclared type answered the scope %q", scope)
	}
}

func TestTheReadScopeFollowsTheEntityTheEventIsAbout(t *testing.T) {
	cases := []struct {
		eventType event.Type
		want      string
	}{
		{event.ItemCompleted, "items:read"},
		{event.CommentCreated, "items:read"},
		{event.ItemLabelAdded, "items:read"},
		{event.RecurrenceOccurrenceCreated, "items:read"},
		{event.ContainerCreated, "containers:read"},
		{event.BucketReordered, "containers:read"},
		{event.LabelDeleted, "containers:read"},
		{event.AttachmentAdded, "media:read"},
		{event.TemplateInstantiated, "templates:read"},
	}
	for _, tc := range cases {
		t.Run(string(tc.eventType), func(t *testing.T) {
			if got := tc.eventType.ReadScope(); got != tc.want {
				t.Errorf("read scope = %q, want %q", got, tc.want)
			}
		})
	}
}
