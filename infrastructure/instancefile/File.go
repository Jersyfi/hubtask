// SPDX-License-Identifier: Apache-2.0
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

// The areas a file may carry. The same five the API answers, so a file is the document an operator
// would have read back — and an area this build does not know is refused rather than ignored,
// because a file is written once and read by every future start: a typo that silently did nothing
// is a configuration somebody believes is in force.
var areas = map[string]bool{
	"sign_in": true, "legal": true, "localisation": true, "quotas": true, "ai": true,
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
		if !areas[name] && name != "blocklist_file" && name != "metering" && name != aiModelsKey && name != plansKey && name != presetsKey &&
			name != platformSubscriptionsKey {
			return nil, false, shared.ErrValidation.
				WithDetail("config.instance_file_area_unknown").
				WithParams(map[string]string{"source": path, "area": name})
		}
	}
	return document, true, nil
}

// aiModelsKey is the list of AI models the installation offers (ADR-0072 §1). Not an area of the
// level: the models are rows of their own, written through their own use case.
const aiModelsKey = "ai_models"

// AiModel is one offered model as the file names it. The key is never in the file - a secret in a
// configuration file is a key in clear - but in the environment variable APIKeyEnv names.
type AiModel struct {
	DisplayName     string
	Processor       string
	Kind            string
	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
	Jurisdiction    string
	// APIKey is the key read from the named variable, and HasKey whether the file named one.
	APIKey string
	HasKey bool
}

// AiModels reads the file's offered models. Present is false where the file says nothing about
// them, which is not the same as an empty list: an enforcing file with `"ai_models": []` withdraws
// every offered model, and one without the key leaves them to the other doors.
func AiModels(
	document map[string]any, lookup func(string) (string, bool), source string,
) (models []AiModel, present bool, err error) {
	raw, present := document[aiModelsKey]
	if !present {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, malformedModels(source)
	}
	for _, entry := range list {
		fields, isObject := entry.(map[string]any)
		if !isObject {
			return nil, true, malformedModels(source)
		}
		text := func(name string) string {
			value, _ := fields[name].(string)
			return value
		}
		model := AiModel{
			DisplayName: text("display_name"), Processor: text("processor"), Kind: text("kind"),
			BaseURL: text("base_url"), CompletionModel: text("completion_model"),
			EmbeddingModel: text("embedding_model"), Jurisdiction: text("jurisdiction"),
		}
		if variable := text("api_key_env"); variable != "" {
			key, set := lookup(variable)
			if !set || key == "" {
				return nil, true, shared.ErrValidation.
					WithDetail("config.instance_file_ai_key_missing").
					WithParams(map[string]string{"source": source, "variable": variable})
			}
			model.APIKey, model.HasKey = key, true
		}
		models = append(models, model)
	}
	return models, true, nil
}

func malformedModels(source string) error {
	return shared.ErrValidation.
		WithDetail("config.instance_file_malformed").
		WithParams(map[string]string{"source": source})
}

// plansKey is the list of plans (multi-tenancy.md §4.2). Not an area of the level: plans are rows
// of their own, each a document in the shape the API takes - `name`, `display_name`, the four
// areas and `left_out` - so a plan copied out of `GET /admin/plans/{name}` goes into the file as it is.
const plansKey = "plans"

// Plans reads the file's plans, each as the document the plan use cases read. Present is false where
// the file says nothing about plans, which is not the same as an empty list: an enforcing file with
// `"plans": []` removes every plan no workspace is on, while one without the key leaves the plans
// that stand as they are - and, enforcing, closes the API's plan writes all the same, as it closes
// every other value of the level.
func Plans(document map[string]any, source string) (plans []map[string]any, present bool, err error) {
	raw, present := document[plansKey]
	if !present {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, malformedPlans(source)
	}
	for _, entry := range list {
		fields, isObject := entry.(map[string]any)
		if !isObject {
			return nil, true, malformedPlans(source)
		}
		if name, _ := fields["name"].(string); name == "" {
			return nil, true, malformedPlans(source)
		}
		plans = append(plans, fields)
	}
	return plans, true, nil
}

func malformedPlans(source string) error {
	return shared.ErrValidation.
		WithDetail("config.instance_file_malformed").
		WithParams(map[string]string{"source": source})
}

// presetsKey is the list of the installation's presets (UC-INS-18). Not an area of the level: the
// presets are rows of their own, each a document in the shape the API takes - `name`, `preset`,
// `settings`, `is_default`, `locks` - so one copied out of `GET /admin/presets` goes into the file
// as it is. A preset is known by its name.
const presetsKey = "presets"

// Presets reads the file's presets, each as the document the preset use cases read. Present is
// false where the file says nothing about them, which is not an empty list: an enforcing file with
// `"presets": []` withdraws every preset.
func Presets(document map[string]any, source string) (presets []map[string]any, present bool, err error) {
	raw, present := document[presetsKey]
	if !present {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, malformedPlans(source)
	}
	names := map[string]bool{}
	for _, entry := range list {
		fields, isObject := entry.(map[string]any)
		if !isObject {
			return nil, true, malformedPlans(source)
		}
		name, _ := fields["name"].(string)
		if name == "" || names[name] {
			return nil, true, malformedPlans(source)
		}
		names[name] = true
		presets = append(presets, fields)
	}
	return presets, true, nil
}

// platformSubscriptionsKey is the installation's subscriptions for a purchase platform
// (UC-INS-14): rows of their own, like the offered models.
const platformSubscriptionsKey = "platform_subscriptions"

// PlatformSubscription is one subscription as the file names it. The secret is never in the file -
// a secret in a configuration file is a secret in clear - but in the file SecretFile names.
type PlatformSubscription struct {
	TargetURL string
	Secret    string
}

// PlatformSubscriptions reads the file's subscriptions, each secret from the file it names.
// Present is false where the file says nothing about them, which is not an empty list: an
// enforcing file with `"platform_subscriptions": []` removes every subscription.
func PlatformSubscriptions(document map[string]any, readFile func(string) ([]byte, error), source string,
) (subscriptions []PlatformSubscription, present bool, err error) {
	raw, present := document[platformSubscriptionsKey]
	if !present {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, malformedPlans(source)
	}
	for _, entry := range list {
		fields, isObject := entry.(map[string]any)
		if !isObject {
			return nil, true, malformedPlans(source)
		}
		target, _ := fields["target_url"].(string)
		secretFile, _ := fields["secret_file"].(string)
		if target == "" || secretFile == "" {
			return nil, true, malformedPlans(source)
		}
		secret, err := readFile(secretFile)
		if err != nil {
			return nil, true, shared.ErrUnavailable.
				WithDetail("config.instance_file_unreadable").
				WithParams(map[string]string{"source": secretFile}).
				WithCause(fmt.Errorf("reading the secret of a platform subscription: %w", err))
		}
		value := strings.TrimSpace(string(secret))
		if value == "" {
			return nil, true, malformedPlans(source)
		}
		subscriptions = append(subscriptions, PlatformSubscription{TargetURL: target, Secret: value})
	}
	return subscriptions, true, nil
}
