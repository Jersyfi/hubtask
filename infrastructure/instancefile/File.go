// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package instancefile is the third door to the instance level (ADR-0070 §5, the concept's §5.4).
//
// The API is the product; `hubctl` and the dashboard are two clients of it, and a file is the
// third — for GitOps and immutable containers, where the thing that owns the configuration is a
// repository rather than a person. Two modes, and the difference is who owns the values afterwards:
//
//   - **`seed`** writes them once, at the first start that finds the level empty, and then leaves
//     it alone. The file is where the installation *began*; the API owns it from then on.
//   - **`enforce`** writes them at every start, and the writing routes refuse in between. The file
//     is the source, permanently, and a save through the API would be one the next restart
//     silently undoes.
//
// **One source per mode, never two.** That sentence is the whole of why `enforce` refuses rather
// than merging: an installation whose values come from two places is one where nobody can say what
// applies without reading both.
//
// **This package maps a file to a request, and nothing else.** It does not know which switch takes
// a number — the use case does, and a second copy of that knowledge here would be a second copy to
// get wrong. "Drei Türen, eine API" is meant literally: the file goes through `WriteInstanceSettings`
// exactly as the dashboard and `hubctl` do, so a value a file may set is a value the API accepts.
//
// JSON rather than YAML, and deliberately: the level is already JSON in the database and in the
// contract, so the file is the same document an operator would `GET` — and the server binary gains
// no dependency for it. `gopkg.in/yaml.v3` is in this repository for tools and tests; putting it on
// every installation's start-up path is a supply-chain decision, not a formatting preference.
package instancefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Mode is which of the two the operator chose.
type Mode string

const (
	// ModeSeed writes the level once, at the first start that finds it empty.
	ModeSeed Mode = "seed"
	// ModeEnforce writes it at every start and refuses the writing routes in between.
	ModeEnforce Mode = "enforce"
)

// ParseMode reads the configured mode. Empty is `seed`, which is the mode that does the least.
func ParseMode(raw string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(raw))) {
	case "", ModeSeed:
		return ModeSeed, nil
	case ModeEnforce:
		return ModeEnforce, nil
	default:
		return "", shared.ErrValidation.
			WithDetail("config.instance_file_mode_invalid").
			WithParams(map[string]string{"mode": raw})
	}
}

// The areas a file may carry. The same four the API answers, so a file is the document an operator
// would have read back — and an area this build does not know is refused rather than ignored,
// because a file is written once and read by every future start: a typo that silently did nothing
// is a configuration somebody believes is in force.
var areas = map[string]bool{
	"sign_in": true, "legal": true, "localisation": true, "quotas": true,
}

// Read parses the file at a path into the request body `WriteInstanceSettings` takes.
//
// A path that names nothing is not an error: an installation may be configured for a file that has
// not been written yet, and refusing to start over it would make the mode useless in exactly the
// deployment it is for.
func Read(path string) (map[string]any, bool, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own configuration
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, shared.ErrUnavailable.
			WithDetail("config.instance_file_unreadable").
			WithCause(fmt.Errorf("reading %q: %w", path, err))
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		// Refused rather than skipped, and this is the one place in the level's handling that
		// fails loudly: a malformed row in the database is one setting nobody can read, and a
		// malformed *file* is an operator whose whole configuration silently did not apply.
		return nil, false, shared.ErrValidation.
			WithDetail("config.instance_file_malformed").
			WithParams(map[string]string{"source": path}).
			WithCause(fmt.Errorf("parsing %q: %w", path, err))
	}

	for name := range document {
		if !areas[name] && name != "blocklist_file" {
			return nil, false, shared.ErrValidation.
				WithDetail("config.instance_file_area_unknown").
				WithParams(map[string]string{"source": path, "area": name})
		}
	}
	return document, true, nil
}
