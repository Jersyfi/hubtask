// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel
package view

import (
	"reflect"
	"testing"
)

// UC-WRK-43: every spelling the reader types is written back to a line that reads the same query.
func TestEveryKindOfQueryIsWrittenBackToItself(t *testing.T) {
	for _, line := range []string{
		`passkey`,
		`"two words" passkey`,
		`who: me`,
		`who: me, none`,
		`-who: me`,
		`status: open, done`,
		`is: open`,
		`is: done is: blocked`,
		`is: blocking`,
		`type: task`,
		`type: page, whiteboard`,
		`label: urgent`,
		`-label: urgent`,
		`due: today`,
		`due: this-week`,
		`due: last-week`,
		`due: next-week`,
		`due: 2026-01-01..2026-02-01`,
		`due: 2026-01-01..+2w`,
		`due: >2026-01-01`,
		`due: <2026-01-01`,
		`due: none`,
		`created: >=2026-01-01`,
		`title: "a phrase"`,
		`title: passkey`,
		`cf.priority: >= 3`,
		`cf.priority: 3`,
		`cf.size: big, small`,
		`(a OR b) -c`,
		`(who: me OR who: none) is: open`,
		`NOT (who: me)`,
		`-(label: a OR label: b)`,
		`in: archived`,
		`is: archived`,
		`is: trashed`,
		`sort: -due`,
		`sort: due, -updated`,
		`who: me sort: title`,
		`plan.critical: true`,
		`time.logged: > 120`,
		`desk.sla: <2026-01-01`,
	} {
		q, err := ParseQuery(line, "/q")
		if err != nil {
			t.Errorf("%q is not read: %v", line, err)
			continue
		}
		formatted, ok := FormatQuery(q)
		if !ok {
			t.Errorf("%q is not written back", line)
			continue
		}
		again, err := ParseQuery(formatted, "/q")
		if err != nil || !reflect.DeepEqual(again, q) {
			t.Errorf("%q -> %q reads otherwise: %v", line, formatted, err)
		}
	}
}
