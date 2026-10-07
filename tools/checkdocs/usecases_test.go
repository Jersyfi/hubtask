// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

const useCaseBody = "\n## Goal\n\nA person keeps a hub private.\n\n## How to check\n\n1. One.\n2. Two.\n3. Three.\n\n## Where it ends\n\nHere.\n"

func TestToday(t *testing.T) {
	cases := []struct {
		name, state, today, want string
	}{
		{"partial with one line per unmet check", "partial", "* Check 2: not met — the screen\n* Check 3: not met in the web app — x\n", ""},
		{"built without Today", "built", "", ""},
		{"verified without Today", "verified", "", ""},
		{"partial without Today", "partial", "", "says in ## Today what is not met yet"},
		{"specified without Today", "specified", "", "says in ## Today what is not met yet"},
		{"built with a Today", "built", "* Check 2: not met — x\n", "a built use case has no ## Today"},
		{"verified with a Today", "verified", "* Check 2: not met — x\n", "a verified use case has no ## Today"},
		{"a line that names no check", "partial", "* Check 2: not met — x\nThe rest is fine.\n", `each line is "* Check n`},
		{"a check that does not exist", "partial", "* Check 7: not met — x\n", "lists check 7, which ## How to check does not have"},
		{"a check listed twice", "partial", "* Check 2: not met — x\n* Check 2: not met in hubctl — y\n", "lists check 2 twice"},
		{"a Today with no check", "partial", "Nothing yet.\n", "names no check"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := useCaseBody
			if c.today != "" {
				body += "\n## Today\n\n" + c.today
			}
			uc := useCase{path: "x.md", id: "UC-ID-16", fields: map[string]string{"state": c.state}, lists: map[string][]string{}, body: body}
			var got []string
			for _, p := range checkUseCase(t.TempDir(), uc, nil, nil, nil) {
				if strings.Contains(p, "Today") {
					got = append(got, p)
				}
			}
			joined := strings.Join(got, "\n")
			if c.want == "" && joined != "" {
				t.Fatalf("want nothing about Today, got:\n%s", joined)
			}
			if c.want != "" && !strings.Contains(joined, c.want) {
				t.Fatalf("want %q, got:\n%s", c.want, joined)
			}
		})
	}
}
