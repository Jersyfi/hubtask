// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package catalogue_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
