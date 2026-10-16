// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"regexp"
	"testing"
)

// The CloudNativePG operator integration runs is the one gate-pitr drills against (ADR-0082): the
// same version and the same checksum, written in two places because a host bootstrap cannot read
// the Makefile. Two pins that drift are an integration that proves an operator nobody tested.
func TestIntegrationPinsTheOperatorGatePitrDrills(t *testing.T) {
	makefile := read(t, "../../Makefile")
	bootstrap := read(t, "../../deploy/integration/bootstrap.sh")

	for _, name := range []string{"CNPG_VERSION", "CNPG_SHA256"} {
		inMake := regexp.MustCompile(`(?m)^` + name + `\s*:=\s*(\S+)`).FindStringSubmatch(makefile)
		inBootstrap := regexp.MustCompile(`(?m)^readonly ` + name + `="([^"]+)"`).FindStringSubmatch(bootstrap)
		if inMake == nil || inBootstrap == nil {
			t.Fatalf("%s is not pinned in both places (Makefile %v, bootstrap.sh %v)", name, inMake, inBootstrap)
		}
		if inMake[1] != inBootstrap[1] {
			t.Errorf("%s: the Makefile pins %s, integration's bootstrap.sh %s", name, inMake[1], inBootstrap[1])
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}
