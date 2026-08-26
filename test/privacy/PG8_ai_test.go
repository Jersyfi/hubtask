// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"strings"
	"testing"
)

// PG-8: third-country AI without explicit confirmation is refused (ADR-0018 decision 7,
// data-protection.md §10).
//
// **There is nothing to gate yet, and that is what this test says.** No AI provider is configured
// by anything in this build: `OutboundConfig` names AI providers in a comment as a future caller,
// and there is no provider, no region, no confirmation and no call. A gate written against that
// absence would be a green nobody may read as a check - which is exactly the failure this whole
// task exists to end.
//
// So it is a **tripwire** rather than a check: it goes red on the day somebody adds a configuration
// surface for an AI provider, and it says what has to arrive with it. Until then it records, in a
// place that runs, that PG-8 belongs to `0.7.0` - named rather than quietly absent.

// aiConfiguration is what an AI surface would look like in the configuration port. If any of these
// appears, the confirmation this gate is about has to appear with it.
var aiConfiguration = []string{
	"AIConfig", "AIProvider", "AI_PROVIDER", "AIRegion", "AIModel",
}

func TestPG8ThirdCountryAIHasNothingToRefuseYet(t *testing.T) {
	config := readFile(t, "../../core/port/environment/Port.go")

	for _, marker := range aiConfiguration {
		if !strings.Contains(config, marker) {
			continue
		}

		t.Errorf("the configuration port names %q, so an AI surface has arrived (PG-8). "+
			"ADR-0018 decision 7 requires an explicit confirmation before a provider outside the "+
			"EEA may be used, and the use is audited with the provider, the region, the model and "+
			"the purpose. Write that check here, in place of this tripwire", marker)
	}

	t.Log("PG-8 has nothing to refuse: this build has no AI provider surface. " +
		"It belongs to 0.7.0 (roadmap), and this tripwire fires when one arrives.")
}
