// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/tools/losttests"
)

// goTests is a Go test file with one test per name, each with a body of its own.
func goTests(names ...string) string {
	var b strings.Builder
	b.WriteString("package x\n\nimport \"testing\"\n")
	for _, name := range names {
		b.WriteString("\nfunc " + name + "(t *testing.T) {\n\tif got := " + strings.ToLower(name) + "(); got != 1 {\n\t\tt.Fatalf(\"" + name + ": %d\", got)\n\t}\n}\n")
	}
	return b.String()
}

// walks is a browser suite with one walk per title.
func walks(titles ...string) string {
	var b strings.Builder
	b.WriteString("import { test } from 'node:test';\n")
	for _, title := range titles {
		b.WriteString("\ntest('" + title + "', async (t) => {\n  await open(t, '" + title + "');\n});\n")
	}
	return b.String()
}

const gone = "\x00" // a file the branch deletes

func TestLostTests(t *testing.T) {
	const (
		goFile = "core/x/x_test.go"
		walk   = "apps/webapp/e2e/board.test.mjs"
		unit   = "packages/sync-engine/src/queue.test.ts"
	)
	three := goTests("TestOne", "TestTwo", "TestThree")
	cases := []struct {
		name        string
		base, head  map[string]string
		description string
		want        string // "" = nothing asked for
	}{
		{"a test's body changed", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "got != 1", "got != 2", 1)}, "", ""},
		{"a test added", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestTwo", "TestThree", "TestFour")}, "", ""},
		{"a test renamed in place", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "func TestTwo(", "func TestTheSecond(", 1)}, "", ""},
		{"a test renamed and its body changed", map[string]string{goFile: three},
			map[string]string{goFile: strings.NewReplacer("func TestTwo(", "func TestTheSecond(", "testtwo()", "second()").Replace(three)}, "", ""},
		{"a test moved within its file", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestTwo", "TestThree", "TestOne")}, "", ""},
		{"a test moved between files", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestThree"), "core/y/y_test.go": goTests("TestTwo")}, "", ""},
		{"a file renamed", map[string]string{goFile: three},
			map[string]string{goFile: gone, "core/x/renamed_test.go": three}, "", ""},
		{"a file split into two", map[string]string{goFile: three},
			map[string]string{goFile: gone, "core/x/a_test.go": goTests("TestOne"), "core/x/b_test.go": goTests("TestTwo", "TestThree")}, "", ""},
		{"a test deleted and not named", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestThree")}, "Removes the second.", `core/x/x_test.go no longer has the test "TestTwo"`},
		{"a test deleted and a longer name named", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestThree")}, "TestTwoOfThem asserts the same thing.", `no longer has the test "TestTwo"`},
		{"a test deleted and named", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestThree")}, "`TestTwo` is deleted: TestOne asserts the same thing.", ""},
		{"a test deleted and named only in a comment", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestOne", "TestThree")}, "<!-- TestTwo -->", `no longer has the test "TestTwo"`},
		{"a fuzz test deleted", map[string]string{goFile: goTests("TestOne", "FuzzParse")},
			map[string]string{goFile: goTests("TestOne")}, "", `no longer has the test "FuzzParse"`},
		{"two deleted, one renamed", map[string]string{goFile: three},
			map[string]string{goFile: goTests("TestUno")}, "", `"TestTwo"|"TestThree"`},
		{"a test file deleted and not named", map[string]string{goFile: three, "core/x/keep_test.go": goTests("TestKept")},
			map[string]string{goFile: gone}, "TestOne, TestTwo and TestThree go.", `the test file core/x/x_test.go is deleted - name "x_test.go"`},
		{"a test file deleted and named", map[string]string{goFile: three, "core/x/keep_test.go": goTests("TestKept")},
			map[string]string{goFile: gone}, "x_test.go is deleted: the behaviour was removed on purpose.", ""},
		{"a file of helpers deleted", map[string]string{"core/x/helpers_test.go": "package x\n\nfunc helper() int { return 1 }\n", goFile: three},
			map[string]string{"core/x/helpers_test.go": gone}, "", `name "helpers_test.go"`},
		{"a skip added", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\tt.Skip(\"flaky\")\n\tif got := testtwo()", 1)}, "", `core/x/x_test.go adds a skip to "TestTwo"`},
		{"a conditional skip added", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\tif testing.Short() {\n\t\tt.Skipf(\"needs %s\", \"docker\")\n\t}\n\tif got := testtwo()", 1)}, "", `adds a skip to "TestTwo"`},
		{"a skip added and named", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\tt.Skip(\"flaky\")\n\tif got := testtwo()", 1)}, "TestTwo is skipped: its behaviour was removed on purpose.", ""},
		{"a skip added to a helper the base had", map[string]string{goFile: "package x\n\nfunc needsDocker(t *testing.T) {\n\tt.Helper()\n}\n" + three},
			map[string]string{goFile: "package x\n\nfunc needsDocker(t *testing.T) {\n\tt.Helper()\n\tt.Skip(\"no docker\")\n}\n" + three}, "", `adds a skip to "needsDocker"`},
		{"a new test that skips without its service", map[string]string{goFile: three},
			map[string]string{goFile: three + "\nfunc TestFour(t *testing.T) {\n\tif testing.Short() {\n\t\tt.Skip(\"needs a database\")\n\t}\n}\n"}, "", ""},
		{"a renamed test that gains a skip", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "func TestTwo(t *testing.T) {\n", "func TestTheSecond(t *testing.T) {\n\tt.Skip(\"later\")\n", 1)}, "", `adds a skip to "TestTheSecond"`},
		{"a skip's message reworded", map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\tt.Skip(\"flaky\")\n\tif got := testtwo()", 1)},
			map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\tt.Skip(\"flaky on arm64\")\n\tif got := testtwo()", 1)}, "", ""},
		{"a skip in a comment", map[string]string{goFile: three},
			map[string]string{goFile: strings.Replace(three, "\tif got := testtwo()", "\t// never t.Skip() here\n\tif got := testtwo()", 1)}, "", ""},
		{"a walk removed", map[string]string{walk: walks("chromium: a card moves", "chromium: a column folds")},
			map[string]string{walk: walks("chromium: a card moves")}, "", `apps/webapp/e2e/board.test.mjs no longer has the test "chromium: a column folds"`},
		{"a walk removed and named", map[string]string{walk: walks("chromium: a card moves", "chromium: a column folds")},
			map[string]string{walk: walks("chromium: a card moves")}, "The walk \"chromium: a column folds\" goes: folding was removed on purpose.", ""},
		{"a walk retitled", map[string]string{walk: walks("chromium: a card moves", "chromium: a column folds")},
			map[string]string{walk: walks("chromium: a card moves", "chromium: a column folds and unfolds")}, "", ""},
		{"a walk moved to another suite", map[string]string{walk: walks("chromium: a card moves", "chromium: a column folds")},
			map[string]string{walk: walks("chromium: a card moves"), "apps/webapp/e2e/columns.test.mjs": walks("chromium: a column folds")}, "", ""},
		{"a suite deleted", map[string]string{walk: walks("chromium: a card moves"), "apps/webapp/e2e/keep.test.mjs": walks("chromium: kept")},
			map[string]string{walk: gone}, "", `name "board.test.mjs"`},
		{"a walk with an inserted width removed", map[string]string{walk: "for (const width of [320, 1280]) {\n  test(`chromium: ${width} px — a job that proposed nothing says why`, async (t) => {\n    await open(t);\n  });\n}\n"},
			map[string]string{walk: "for (const width of [320, 1280]) {\n}\n"}, "", `no longer has the test "px — a job that proposed nothing says why"`},
		{"a walk known only by inserted values removed", map[string]string{walk: "for (const c of cases) {\n  test(`${c.name}: ${c.width}`, async (t) => {\n    await open(t);\n  });\n}\n" + walks("chromium: kept")},
			map[string]string{walk: walks("chromium: kept")}, "", `no longer has the test "board.test.mjs"`},
		{"a unit test removed", map[string]string{unit: walks("the queue keeps its order", "a retry waits")},
			map[string]string{unit: walks("the queue keeps its order")}, "", `packages/sync-engine/src/queue.test.ts no longer has the test "a retry waits"`},
		{"a walk skipped by its options", map[string]string{walk: walks("chromium: a card moves")},
			map[string]string{walk: strings.Replace(walks("chromium: a card moves"), "', async (t)", "', { skip: true }, async (t)", 1)}, "", `adds a skip to "chromium: a card moves"`},
		{"a walk made a todo", map[string]string{walk: walks("chromium: a card moves")},
			map[string]string{walk: strings.Replace(walks("chromium: a card moves"), "test('", "test.todo('", 1)}, "", `adds a skip to "chromium: a card moves"`},
		{"a walk skipped in its body", map[string]string{walk: walks("chromium: a card moves")},
			map[string]string{walk: strings.Replace(walks("chromium: a card moves"), "  await open", "  if (!chromium) return t.skip('no browser');\n  await open", 1)}, "", `adds a skip to "chromium: a card moves"`},
		{"a new walk that skips without a build", map[string]string{walk: walks("chromium: a card moves")},
			map[string]string{walk: walks("chromium: a card moves") + "\ntest('chromium: the bundle is small', { skip: !built && 'no build' }, () => {});\n"}, "", ""},
		{"a file that is no test", map[string]string{"core/x/x.go": "package x\n\nfunc TestLike() {}\n"},
			map[string]string{"core/x/x.go": "package x\n"}, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newScratchRepo(t)
			r.write("README.md", "x\n")
			for path, content := range c.base {
				r.write(path, content)
			}
			r.commit("base")
			r.run("checkout", "-q", "-b", "work")
			for path, content := range c.head {
				if content == gone {
					r.remove(path)
				} else {
					r.write(path, content)
				}
			}
			r.commit("work")

			facts, err := readHistory(r.dir, "main", "work", time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(historyProblems(c.description, facts), "\n")
			if c.want == "" {
				if got != "" {
					t.Fatalf("asked for:\n%s", got)
				}
				return
			}
			for _, want := range strings.Split(c.want, "|") {
				if !strings.Contains(got, want) {
					t.Errorf("want a problem containing %q, got:\n%s", want, got)
				}
			}
			if !strings.Contains(got, "the behaviour it asserts was removed or changed on purpose, or another test asserts the same thing") {
				t.Errorf("the problem does not give the two reasons:\n%s", got)
			}
		})
	}
}

// What main brought into the branch by a merge is not the branch's: a test main removed is read
// from the merge base, which the merge moves.
func TestLostTestsReadFromTheMergeBase(t *testing.T) {
	r := newScratchRepo(t)
	r.write("core/x/x_test.go", goTests("TestOne", "TestTwo"))
	r.commit("base")
	r.run("checkout", "-q", "-b", "work")
	r.write("core/y/y_test.go", goTests("TestY"))
	r.commit("work")
	r.run("checkout", "-q", "main")
	r.write("core/x/x_test.go", goTests("TestOne"))
	r.commit("main removes a test")
	r.run("checkout", "-q", "work")
	r.run("merge", "-q", "--no-edit", "main")

	facts, err := readHistory(r.dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.testsLost) != 0 {
		t.Fatalf("the branch answers for what main removed: %+v", facts.testsLost)
	}
}

// A pull request opened before the rule is not held to it.
func TestLostTestsHoldFromTheirDate(t *testing.T) {
	facts := branchFacts{known: true, testsLost: []losttests.Test{{Path: "core/x/x_test.go", Name: "TestTwo", How: "removed", Identifier: true}}}
	if got := historyProblems("", facts); len(got) != 1 {
		t.Fatalf("a local run is not held to the rule: %v", got)
	}
	facts.opened = testsSince.Add(-time.Hour)
	if got := historyProblems("", facts); len(got) != 0 {
		t.Fatalf("a pull request opened before the rule is held to it: %v", got)
	}
}
