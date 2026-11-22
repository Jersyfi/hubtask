// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package losttests

import (
	"os"
	"reflect"
	"testing"
)

// The fixture is the diff of merge commit c3a5e93d5 reduced to three files: a walk removed beside
// four added further down, a test file deleted, and a Go test renamed in place.
func TestInReadsAMergeCommit(t *testing.T) {
	diff, err := os.ReadFile("testdata/merge-c3a5e93d5.diff")
	if err != nil {
		t.Fatal(err)
	}
	got := In(string(diff), map[string]bool{"apps/webapp/src/lib/search/kinds.test.ts": true})
	want := []Test{
		{Path: "apps/webapp/src/lib/search/kinds.test.ts", Name: "kinds.test.ts", How: "deleted"},
		{Path: "apps/webapp/src/lib/servicedesk/queues.test.ts", Name: "a hub's desks are its collections offering a type open to the portal", How: "removed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestIn(t *testing.T) {
	const goHead = "diff --git a/core/a/a_test.go b/core/a/a_test.go\n--- a/core/a/a_test.go\n+++ b/core/a/a_test.go\n@@ -1,9 +1,9 @@\n"
	const jsHead = "diff --git a/apps/webapp/e2e/a.test.mjs b/apps/webapp/e2e/a.test.mjs\n--- a/apps/webapp/e2e/a.test.mjs\n+++ b/apps/webapp/e2e/a.test.mjs\n@@ -1,9 +1,9 @@\n"
	cases := []struct {
		name, diff string
		want       []Test
	}{
		{"a walk turned into a skipped one", jsHead + "-test('the board opens', async (t) => {\n+test.skip('the board opens', async (t) => {\n   await open(t);\n",
			[]Test{{Path: "apps/webapp/e2e/a.test.mjs", Name: "the board opens", How: "skipped"}}},
		{"two tests removed beside one new one: the first is the rename, the second is gone",
			goHead + "-func TestOne(t *testing.T) {\n-}\n-func TestTwo(t *testing.T) {\n-}\n+func TestFresh(t *testing.T) {\n+}\n",
			[]Test{{Path: "core/a/a_test.go", Name: "TestTwo", How: "removed", Identifier: true}}},
		{"a skip added inside a test the base had is named by that test",
			goHead + " func TestKept(t *testing.T) {\n+\tif testing.Short() {\n+\t\tt.Skip(\"slow\")\n+\t}\n \trun(t)\n }\n",
			[]Test{{Path: "core/a/a_test.go", Name: "TestKept", How: "skipped", Identifier: true}}},
		{"a skip inside a test the change adds is not proof lost",
			goHead + " func TestKept(t *testing.T) {\n }\n+func TestNew(t *testing.T) {\n+\tt.Skip(\"needs a container\")\n+}\n", nil},
		{"a comment that speaks of a skip adds none",
			goHead + " func TestKept(t *testing.T) {\n+\t// t.Skip is not what this does\n }\n", nil},
		{"a test moved to another file",
			goHead + "-func TestMoved(t *testing.T) {\n-}\n diff --git a/core/b/b_test.go b/core/b/b_test.go\n--- a/core/b/b_test.go\n+++ b/core/b/b_test.go\n@@ -1,2 +1,4 @@\n package b\n+func TestMoved(t *testing.T) {\n+}\n", nil},
		{"a skip added to a helper whose declaration changed is named by the helper",
			goHead + "-func requireStack(t *testing.T) {\n+func requireStack(t *testing.T, name string) {\n+\tt.Skip(\"no stack\")\n \tstart(t)\n }\n",
			[]Test{{Path: "core/a/a_test.go", Name: "requireStack", How: "skipped", Identifier: true}}},
		{"a skip inside a helper the change adds is not proof lost",
			goHead + " func TestKept(t *testing.T) {\n }\n+func requireStack(t *testing.T) {\n+\tt.Skip(\"no stack\")\n+}\n", nil},
		{"a title too short to name its test does not come back as another file's short title",
			jsHead + "-test(`case ${kind}`, () => {\n-});\n diff --git a/apps/webapp/e2e/b.test.mjs b/apps/webapp/e2e/b.test.mjs\n--- a/apps/webapp/e2e/b.test.mjs\n+++ b/apps/webapp/e2e/b.test.mjs\n@@ -1,2 +1,4 @@\n import x from 'x';\n+test(`row ${id}`, () => {\n+});\n",
			[]Test{{Path: "apps/webapp/e2e/a.test.mjs", Name: "a.test.mjs", How: "removed"}}},
		{"a title too short to name its test, changed in place, is a rename",
			jsHead + "-test(`case ${kind}`, () => {\n+test(`case ${kind} x`, () => {\n });\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := In(c.diff, nil); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestNamedIn(t *testing.T) {
	body := "`TestGone` asserted the old retry. The walk 'the board opens' moved."
	for _, c := range []struct {
		test Test
		want bool
	}{
		{Test{Name: "TestGone", Identifier: true}, true},
		{Test{Name: "TestGo", Identifier: true}, false},
		{Test{Name: "the board opens"}, true},
		{Test{Name: "the board closes"}, false},
	} {
		if got := c.test.NamedIn(body); got != c.want {
			t.Errorf("%q named: %v, want %v", c.test.Name, got, c.want)
		}
	}
}
