// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package forge

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INT-13 check 2: a key is read from a branch's name, a commit's message and a pull request's
// title, standing alone, each once.
func TestKeysInReadsTheKeysAForgeNames(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  []string
	}{
		{"branch", []string{"feature/WEB-42-login"}, []string{"WEB-42"}},
		{"message", []string{"Fix the login (WEB-42) and web-7"}, []string{"WEB-42", "WEB-7"}},
		{"bracketed", []string{"[sup-128] answer"}, []string{"SUP-128"}},
		{"glued in front", []string{"4WEB-42"}, nil},
		{"longer number", []string{"WEB-421"}, []string{"WEB-421"}},
		{"zero", []string{"WEB-0"}, nil},
		{"once", []string{"WEB-42", "web-42 again"}, []string{"WEB-42"}},
		{"too short a handle", []string{"W-42"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, k := range KeysIn(c.texts...) {
				got = append(got, k.String())
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("KeysIn(%q) = %v, want %v", c.texts, got, c.want)
			}
		})
	}
}

func TestKeysInStopsAtItsBound(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		b.WriteString(" WEB-")
		b.WriteString(strings.Repeat("1", i))
	}
	if n := len(KeysIn(b.String())); n != MaxKeysPerSignal {
		t.Fatalf("keys = %d, want %d", n, MaxKeysPerSignal)
	}
}

func TestCheckHookRefusesAnUnknownKindAndAnEmptyName(t *testing.T) {
	cases := []struct {
		kind Kind
		name string
		code string
	}{
		{"SVN", "repo", "forge.kind_unknown"},
		{GitHub, "  ", "forge.hook_name_required"},
		{GitLab, strings.Repeat("a", MaxHookNameRunes+1), "forge.hook_name_too_long"},
	}
	for _, c := range cases {
		_, err := CheckHook(c.kind, c.name)
		var de *shared.Error
		if !errors.As(err, &de) || de.DetailCode != c.code {
			t.Fatalf("CheckHook(%q, %q) = %v, want %s", c.kind, c.name, err, c.code)
		}
	}
	if name, err := CheckHook(Forgejo, "  web  "); err != nil || name != "web" {
		t.Fatalf("CheckHook = %q, %v", name, err)
	}
}

func TestCheckSignalBoundsWhatIsKept(t *testing.T) {
	sig, err := CheckSignal(Signal{
		Kind: Commit, Repository: "team/web", Ref: "abc", Title: "Fix WEB-42\n\nlong body",
		URL: "javascript:alert(1)", State: Open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sig.Title != "Fix WEB-42" || sig.URL != "" || sig.State != Pushed {
		t.Fatalf("signal = %+v", sig)
	}
	if _, err := CheckSignal(Signal{Kind: Branch, Repository: "team/web"}); err == nil {
		t.Fatal("a signal without a reference was kept")
	}
	if _, err := CheckSignal(Signal{Kind: "TAG", Repository: "r", Ref: "v1"}); err == nil {
		t.Fatal("an unknown kind was kept")
	}
	pr, _ := CheckSignal(Signal{Kind: PullRequest, Repository: "r", Ref: "7", State: "WHATEVER"})
	if pr.State != Open || pr.Title != "7" {
		t.Fatalf("pull request = %+v", pr)
	}
}

// UC-INT-13 check 3: the state follows the forge's own moment; a merge is never undone.
func TestApplyFollowsTheForgesMomentAndKeepsAMerge(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	at := func(m int) *time.Time { t := now.Add(time.Duration(m) * time.Minute); return &t }
	open := Signal{Kind: PullRequest, Repository: "r", Ref: "7", Title: "WEB-42 login", State: Open, At: at(-10)}
	link, changed := Apply(Link{}, open, now)
	if !changed || link.State != Open {
		t.Fatalf("first = %+v, %v", link, changed)
	}
	link.ID = shared.ID("l1")
	if _, changed := Apply(link, open, now); changed {
		t.Fatal("the same signal changed the link")
	}
	merged := open
	merged.State, merged.At = Merged, at(-5)
	link, changed = Apply(link, merged, now)
	if !changed || link.State != Merged {
		t.Fatalf("merged = %+v, %v", link, changed)
	}
	late := open
	late.At = at(-1)
	if again, changed := Apply(link, late, now); changed || again.State != Merged {
		t.Fatalf("a later open undid the merge: %+v", again)
	}
	closedBranch := Link{ID: "b", Kind: Branch, State: Open, HappenedAt: *at(-1)}
	old := Signal{Kind: Branch, Repository: "r", Ref: "x", Title: "x", State: Closed, At: at(-3)}
	if _, changed := Apply(closedBranch, old, now); changed {
		t.Fatal("an older signal changed the link")
	}
}
