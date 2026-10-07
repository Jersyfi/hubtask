// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A numbered section of a subject document is cited from code, ADRs and other documents by its
// number, and those citations outlive the text around them. So a section's number never goes away
// and never moves: a section may be retitled and rewritten, and one whose content went elsewhere
// keeps its heading with a sentence saying where. What this cannot
// see is a number kept for other content than it had - a citation then still resolves, to the wrong
// thing; that stays with review.

// sectionsSince is when the rule took effect.
var sectionsSince = readinessSince

var (
	subjectDocument = regexp.MustCompile(`^docs/(?:architecture|design)/[^/]+\.md$`)
	numberedHeading = regexp.MustCompile(`^(#{2,6})\s+(\d+(?:\.\d+)*)\.?\s`)
	fence           = regexp.MustCompile("^\\s*(```|~~~)")
)

// lostSection is a numbered section of a subject document that the merge base had and the head
// no longer carries, or carries as a bare heading.
type lostSection struct {
	path, number string
	bare         bool
}

// numberedSections maps each section number of a document to what stands under its heading, up to
// the next heading of the same or a higher level - its subsections included. Headings inside a
// code fence are text, not sections.
func numberedSections(doc string) map[string]string {
	type open struct {
		number string
		level  int
		body   strings.Builder
	}
	out := map[string]string{}
	var stack []*open
	closeTo := func(level int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			top := stack[len(stack)-1]
			out[top.number] += top.body.String()
			stack = stack[:len(stack)-1]
		}
	}
	inFence := false
	for _, line := range strings.Split(doc, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		var opened *open
		if level := headingLevel(line); level > 0 && !inFence {
			closeTo(level)
			if m := numberedHeading.FindStringSubmatch(line); m != nil {
				opened = &open{number: m[2], level: level}
				if _, seen := out[m[2]]; !seen {
					out[m[2]] = ""
				}
			}
		}
		for _, s := range stack {
			s.body.WriteString(line)
			s.body.WriteString("\n")
		}
		if opened != nil {
			stack = append(stack, opened)
		}
	}
	closeTo(1)
	return out
}

// headingLevel is the number of #s of an ATX heading line, or 0.
func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(line) && line[n] != ' ' && line[n] != '\t') {
		return 0
	}
	return n
}

// lostSections compares the numbered sections of a subject document at the merge base with the
// head; after is "" for a deleted document.
func lostSections(path, before, after string) []lostSection {
	was, is := numberedSections(before), numberedSections(after)
	var out []lostSection
	for _, number := range sortedNumbers(was) {
		body, kept := is[number]
		switch {
		case !kept:
			out = append(out, lostSection{path: path, number: number})
		case strings.TrimSpace(body) == "" && strings.TrimSpace(was[number]) != "":
			out = append(out, lostSection{path: path, number: number, bare: true})
		}
	}
	return out
}

func sortedNumbers(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	// Numeric order per component, so a message lists 2 before 10.
	slices.SortFunc(out, func(a, b string) int {
		as, bs := strings.Split(a, "."), strings.Split(b, ".")
		for i := 0; i < len(as) && i < len(bs); i++ {
			x, _ := strconv.Atoi(as[i])
			y, _ := strconv.Atoi(bs[i])
			if x != y {
				return x - y
			}
		}
		return len(as) - len(bs)
	})
	return out
}

// renumberedSections reads every subject document the branch changes or deletes at the merge base
// and at the head.
func renumberedSections(root, base, head, nameStatus string) ([]lostSection, error) {
	var out []lostSection
	mergeBase := ""
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "M" && fields[0] != "D") || !subjectDocument.MatchString(fields[1]) {
			continue
		}
		if mergeBase == "" {
			found, err := git(root, "merge-base", base, head)
			if err != nil {
				return nil, err
			}
			mergeBase = strings.TrimSpace(found)
		}
		before, err := git(root, "show", mergeBase+":"+fields[1])
		if err != nil {
			return nil, err
		}
		after := ""
		if fields[0] == "M" {
			if after, err = git(root, "show", head+":"+fields[1]); err != nil {
				return nil, err
			}
		}
		out = append(out, lostSections(fields[1], before, after)...)
	}
	return out, nil
}

func sectionProblems(lost []lostSection) []string {
	var problems []string
	for _, l := range lost {
		if l.bare {
			problems = append(problems, fmt.Sprintf("%s keeps §%s as a bare heading - a section whose content moved says so in one sentence under its heading", l.path, l.number))
			continue
		}
		problems = append(problems, fmt.Sprintf("%s no longer has §%s - a section number is cited from elsewhere and is never removed or renumbered; retitle it, or keep the heading with one sentence saying where its content went, and give new content the next free number", l.path, l.number))
	}
	return problems
}
