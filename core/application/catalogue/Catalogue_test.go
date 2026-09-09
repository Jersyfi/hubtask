// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package catalogue_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The catalogue is a list, so what is worth testing about it is what a list can get wrong: that it
// builds, that it holds each use case once, and that reading it cannot change it. What is *in* it
// is the architecture gate's question (test/architecture/parity_test.go), which reconciles this
// list with the source tree, with cmd/server and with the router.

func TestTheCatalogueBuildsARegistry(t *testing.T) {
	registry, err := usecase.NewRegistry(nil, catalogue.Descriptors()...)
	if err != nil {
		// The registry refuses an incomplete entry - a missing summary, a missing audit
		// declaration, a missing handler - so this failing means a use case would stop the server
		// at startup rather than being caught here.
		t.Fatalf("the catalogue does not build: %v", err)
	}
	if len(registry.All()) != len(catalogue.Descriptors()) {
		t.Errorf("%d of %d use cases reached the registry",
			len(registry.All()), len(catalogue.Descriptors()))
	}
}

func TestEveryUseCaseAppearsOnce(t *testing.T) {
	seen := map[string]int{}
	for _, descriptor := range catalogue.Descriptors() {
		seen[descriptor.Name]++
	}
	for name, times := range seen {
		if times != 1 {
			t.Errorf("%s is in the catalogue %d times", name, times)
		}
	}
}

// Reading the catalogue must not be able to change it. Two callers read it - a gate and the event
// matrix generator - and one that sorted the slice in place would change what the other saw.
func TestReadingTheCatalogueDoesNotChangeIt(t *testing.T) {
	first := catalogue.Descriptors()
	if len(first) == 0 {
		t.Fatal("the catalogue is empty")
	}

	first[0] = usecase.Descriptor{Name: "Tampered"}

	if catalogue.Descriptors()[0].Name == "Tampered" {
		t.Error("a caller changed the catalogue by reading it")
	}
}

// Scopes is derived rather than written down, so what is worth testing is the derivation: that it
// is the descriptors' own set, sorted, without repeats, and without the empty scope an operation
// that needs none declares.
func TestScopesAreTheDescriptorsOwnSetSortedAndUnique(t *testing.T) {
	scopes := catalogue.Scopes()
	if len(scopes) == 0 {
		t.Fatal("the build declares no scope at all")
	}
	if !slices.IsSorted(scopes) {
		t.Errorf("the scopes are not sorted: %v", scopes)
	}

	seen := map[string]bool{}
	for _, scope := range scopes {
		if scope == "" {
			t.Error("the empty scope is in the list; an operation that needs none declares it")
		}
		if seen[scope] {
			t.Errorf("%s appears twice", scope)
		}
		seen[scope] = true
	}

	// Every scope a use case asks for has to be one a token can be minted with. The reverse is
	// what CreateAccessToken refuses: a name nothing checks is a bound nothing applies.
	for _, descriptor := range catalogue.Descriptors() {
		if descriptor.TokenScope != "" && !seen[descriptor.TokenScope] {
			t.Errorf("%s needs %s, which no token could carry",
				descriptor.Name, descriptor.TokenScope)
		}
	}
}

// Sessions never carry the control plane (H-06, 0.6.0 decision 6), nor the agent capability
// (J-14): whatever this build declares, SessionScopes leaves those two out - and changes nothing
// else.
func TestSessionScopesLeaveOutTheControlPlaneAndTheAgentCapability(t *testing.T) {
	session := catalogue.SessionScopes()
	for _, scope := range session {
		if strings.HasPrefix(scope, "admin:") {
			t.Errorf("a session carries %q", scope)
		}
		// A session is a person at a keyboard, never an actor this system calls an agent - so the
		// scope would be one a session could carry and nothing would ever read.
		if scope == usecase.AgentDestructiveScope {
			t.Errorf("a session carries %q", scope)
		}
	}

	kept := make(map[string]bool, len(session))
	for _, scope := range session {
		kept[scope] = true
	}
	for _, scope := range catalogue.Scopes() {
		if strings.HasPrefix(scope, "admin:") || scope == usecase.AgentDestructiveScope {
			continue
		}
		if !kept[scope] {
			t.Errorf("SessionScopes dropped %q, which is neither the control plane's nor the agent's", scope)
		}
	}
}

// The agent capability is mintable. A scope a token cannot be created with is a guardrail nobody
// can switch off, which sounds safe and means the feature does not exist: every destructive use
// case would be permanently closed to agents, and the first person to notice would work around it.
func TestTheAgentCapabilityCanBeMinted(t *testing.T) {
	if !slices.Contains(catalogue.Scopes(), usecase.AgentDestructiveScope) {
		t.Errorf("%q is not a scope a token can be minted with", usecase.AgentDestructiveScope)
	}
}

// A hint can never say safe where the server would say no (J-14). `readOnlyHint` and
// `destructiveHint` are what an agent's client decides whether to ask for confirmation on, so a
// descriptor claiming both would be one whose client asks nothing and whose server refuses.
func TestNoDescriptorIsBothReadOnlyAndDestructive(t *testing.T) {
	for _, descriptor := range catalogue.Descriptors() {
		if descriptor.ReadOnly && descriptor.Destructive {
			t.Errorf("%s is announced as read-only and enforced as destructive", descriptor.Name)
		}
	}
}

// Every descriptor marked destructive is closed to an agent, rather than one example of one.
//
// What this catches is the use case somebody adds next: it goes in the catalogue, it is marked, and
// it is closed - with nothing for anybody to remember. And the other direction too, because a
// guardrail that also refused ordinary reads would be one somebody switches off wholesale.
func TestEveryDestructiveUseCaseIsClosedToAnAgent(t *testing.T) {
	agent := appshared.ActorContext{
		Kind:      appshared.ActorAIAgent,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"items:write"},
	}
	permitted := agent
	permitted.Scopes = append(slices.Clone(agent.Scopes), usecase.AgentDestructiveScope)

	destructive := 0
	for _, descriptor := range catalogue.Descriptors() {
		if !descriptor.Destructive {
			if err := descriptor.PermitAgent(agent); err != nil {
				t.Errorf("%s is not destructive and was refused: %v", descriptor.Name, err)
			}
			continue
		}
		destructive++

		if err := descriptor.PermitAgent(agent); err == nil {
			t.Errorf("%s is destructive and open to an agent that holds no capability", descriptor.Name)
		}
		if err := descriptor.PermitAgent(permitted); err != nil {
			t.Errorf("%s is closed to an agent that holds the capability: %v", descriptor.Name, err)
		}
	}

	// A catalogue with nothing destructive in it would make every assertion above vacuous.
	if destructive == 0 {
		t.Fatal("no descriptor is marked destructive, so this test proved nothing")
	}
	t.Logf("%d destructive use cases are closed to an agent by default", destructive)
}
