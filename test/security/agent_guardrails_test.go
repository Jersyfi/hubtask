// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The agent's guardrails as a gate (J-14, ai-first.md §1.3).
//
// It is here rather than only beside the code because what it protects is a *surface*: the claim is
// not "this function refuses" but "there is no destructive use case an agent can reach without the
// capability", and that claim has to be re-checked every time somebody adds one.

func agent(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorAIAgent, TenantID: tenantID, AccountID: actorID,
		Scopes: append([]string{"items:write", "containers:write", "admin:tenants"}, scopes...),
	}
}

// SG: every destructive use case in the catalogue is closed to an agent token that does not carry
// the capability, and open to one that does.
//
// Walked rather than sampled. One example would pass for ever while somebody adds the nineteenth
// destructive use case beside it.
func TestSGAnAgentReachesNoDestructiveUseCaseWithoutTheCapability(t *testing.T) {
	closed, opened := 0, 0

	for _, descriptor := range catalogue.Descriptors() {
		if !descriptor.Destructive {
			continue
		}
		closed++

		err := descriptor.PermitAgent(agent())
		var refusal *domain.Error
		if !errors.As(err, &refusal) {
			t.Errorf("%s is destructive and open to an agent with every scope but the capability",
				descriptor.Name)
			continue
		}
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s refuses an agent with %v, want a forbidden", descriptor.Name, err)
		}
		// Actionable, or an operator reads it as a bug and works around it.
		if refusal.Params["scope"] != usecase.AgentDestructiveScope ||
			refusal.Params["use_case"] != descriptor.Name {
			t.Errorf("%s refuses without saying what to change: %v", descriptor.Name, refusal.Params)
		}

		if err := descriptor.PermitAgent(agent(usecase.AgentDestructiveScope)); err != nil {
			t.Errorf("%s stays closed to an agent that holds the capability: %v", descriptor.Name, err)
			continue
		}
		opened++
	}

	if closed == 0 {
		t.Fatal("no descriptor is marked destructive, so this gate proved nothing")
	}
	if opened != closed {
		t.Errorf("%d of %d destructive use cases open with the capability", opened, closed)
	}
}

// SG: the capability is not something a session picks up by signing in. It is a property of a
// deliberately minted credential, which is what makes "enabled explicitly" true.
func TestSGSigningInDoesNotGrantTheAgentCapability(t *testing.T) {
	for _, scope := range catalogue.SessionScopes() {
		if scope == usecase.AgentDestructiveScope {
			t.Fatal("signing in carries the agent capability")
		}
	}
}

// SG: nothing in the rate limiter or the quota guard branches on the actor being an agent.
//
// ai-first.md §1.3 says rate limits and quotas apply to agents just like to any other token, and the
// way that stays true is that neither mechanism can *see* what kind of actor it is dealing with. A
// behavioural test proves it for the paths it walks; this proves it for the ones nobody wrote a test
// for, which is where an exemption would be added.
func TestSGNeitherTheRateLimitNorTheQuotaKnowsAboutAgents(t *testing.T) {
	for _, dir := range []string{
		"../../core/application/service/quota",
		"../../presentation/rest",
	} {
		forEachSourceFile(t, dir, func(path string, file *ast.File, fset *token.FileSet) {
			if !mentionsLimiting(path) {
				return
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "ActorAIAgent" {
					return true
				}
				t.Errorf("%s decides a limit differently for an agent (ai-first.md §1.3)",
					filepath.Base(path))
				return true
			})
		})
	}
}

// mentionsLimiting picks the files this gate is about by name, so that adding an unrelated file to
// `presentation/rest` does not enlarge what it forbids.
func mentionsLimiting(path string) bool {
	name := filepath.Base(path)
	return strings.Contains(name, "RateLimit") || strings.Contains(name, "Quota") ||
		strings.Contains(name, "Guard")
}

func forEachSourceFile(
	t *testing.T, dir string, visit func(string, *ast.File, *token.FileSet),
) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		visit(path, file, fset)
	}
}
