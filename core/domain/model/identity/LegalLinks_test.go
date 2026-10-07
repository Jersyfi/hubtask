// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The four links resolve workspace -> instance -> nothing, and a locked one is the instance's
// whatever the workspace holds.
func TestTheLegalLinksResolveAndLock(t *testing.T) {
	instance := LegalLayer{
		Links: LegalLinks{
			ImprintURL: "https://host.example/imprint",
			PrivacyURL: "https://host.example/privacy",
			TermsURL:   "https://host.example/terms",
		},
		Locks: map[LegalLink]bool{LinkPrivacy: true},
	}
	workspace := LegalLayer{Links: LegalLinks{
		ImprintURL: "https://acme.example/imprint",
		PrivacyURL: "https://acme.example/privacy",
	}}

	resolved, locks := EffectiveLegal(instance, workspace)

	if resolved.Of(LinkImprint) != "https://acme.example/imprint" {
		t.Errorf("the open link resolved to %q", resolved.Of(LinkImprint))
	}
	if resolved.Of(LinkPrivacy) != "https://host.example/privacy" {
		t.Errorf("the locked link resolved to %q", resolved.Of(LinkPrivacy))
	}
	if resolved.Of(LinkTerms) != "https://host.example/terms" {
		t.Errorf("a link only the instance set resolved to %q", resolved.Of(LinkTerms))
	}
	if resolved.Of(LinkAccessibility) != "" {
		t.Errorf("a link nobody set resolved to %q", resolved.Of(LinkAccessibility))
	}
	if locks[LinkPrivacy] != LockInstance || locks[LinkImprint] != LockNone {
		t.Errorf("the locks read %v", locks)
	}
}

// A private installation owes nobody an imprint: with nothing set anywhere, there is no line.
func TestAnInstallationThatSetNothingShowsNothing(t *testing.T) {
	resolved, locks := EffectiveLegal(LegalLayer{}, LegalLayer{})

	if !resolved.IsEmpty() {
		t.Errorf("something was invented: %+v", resolved)
	}
	if len(locks) != 0 {
		t.Errorf("a lock was invented: %v", locks)
	}
	if (LegalLinks{TermsURL: "https://acme.example/terms"}).IsEmpty() {
		t.Error("one link is not nothing")
	}
}

// With names them by name, which is what a patch does one link at a time.
func TestALinkIsAddressableByName(t *testing.T) {
	links := LegalLinks{}
	for _, name := range LegalLinkNames() {
		links = links.With(name, "https://acme.example/"+string(name))
	}
	for _, name := range LegalLinkNames() {
		if links.Of(name) != "https://acme.example/"+string(name) {
			t.Errorf("%s reads %q", name, links.Of(name))
		}
	}
	if links.Of("something_else") != "" {
		t.Error("a name nobody declared answered something")
	}
	if (LegalLinks{}).With("something_else", "x") != (LegalLinks{}) {
		t.Error("a name nobody declared was written")
	}
}

// A link a sign-in screen renders is a link somebody clicks, so `javascript:` in an
// administrator's hands must not become a cross-site script in everybody else's.
func TestALegalLinkIsCheckedBeforeItIsStored(t *testing.T) {
	for _, accepted := range []string{"", "  ", "https://acme.example/imprint", "http://localhost:9000/x"} {
		if _, err := ValidLegalURL(LinkImprint, accepted); err != nil {
			t.Errorf("%q was refused: %v", accepted, err)
		}
	}
	if answered, _ := ValidLegalURL(LinkImprint, "  https://acme.example/imprint  "); answered != "https://acme.example/imprint" {
		t.Errorf("the stored value is %q, want it trimmed", answered)
	}

	for _, refused := range []string{
		"javascript:alert(1)", "data:text/html,x", "file:///etc/passwd", "not a url at all",
		"//acme.example/imprint",
	} {
		err := mustRefuse(t, refused)
		if !strings.HasPrefix(err.Fields[0].Path, "/sign_in_policy/") {
			t.Errorf("%q was refused against %q", refused, err.Fields[0].Path)
		}
	}

	long := "https://acme.example/" + strings.Repeat("x", MaxLegalURLLength)
	if _, err := ValidLegalURL(LinkImprint, long); err == nil {
		t.Error("a link longer than the bound was accepted")
	}
}

func mustRefuse(t *testing.T, raw string) *shared.Error {
	t.Helper()
	_, err := ValidLegalURL(LinkImprint, raw)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 {
		t.Fatalf("%q answered %v", raw, err)
	}
	return refusal
}

// A link says where it came from, in the order it resolves (UC-ID-12 check 4).
func TestALinkSaysWhichLevelSetIt(t *testing.T) {
	instance := LegalLayer{
		Links: LegalLinks{ImprintURL: "https://host.example/imprint", PrivacyURL: "https://host.example/privacy"},
		Locks: map[LegalLink]bool{LinkImprint: true, LinkTerms: true},
	}
	workspace := LegalLayer{Links: LegalLinks{ImprintURL: "https://acme.example/imprint", PrivacyURL: "https://acme.example/privacy"}}
	for name, want := range map[LegalLink]PolicySource{
		LinkImprint:       SourceInstance,  // locked, whatever the workspace holds
		LinkPrivacy:       SourceWorkspace, // the workspace's own
		LinkTerms:         SourceInstance,  // locked to "no link"
		LinkAccessibility: SourceDefault,   // nobody set one
	} {
		if got := LegalSourceOf(name, instance, workspace); got != want {
			t.Errorf("%s comes from %q, want %q", name, got, want)
		}
	}
}
