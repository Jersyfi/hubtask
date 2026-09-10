// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// automation.md §1.3's three, each with its own prompt and its own audit action - and all three
// queueing rather than calling, because an AI call is somebody else's machine.
func TestEachActionAsksItsOwnQuestion(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		run    func(asker) error
		prompt string
	}{
		{"AI_SUGGEST_FIELDS", func(a asker) error {
			return AiSuggestFields(a.ask).Execute(context.Background(), person(), targetID, false)
		}, "suggest-fields"},
		{"AI_SUMMARIZE", func(a asker) error {
			return AiSummarize(a.ask).Execute(context.Background(), person(), targetID, false)
		}, "summarize"},
		{"AI_CLASSIFY", func(a asker) error {
			return AiClassify(a.ask).Execute(context.Background(), person(), targetID, false)
		}, "classify"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			a := newAsker(true)

			if err := testCase.run(a); err != nil {
				t.Fatalf("asking: %v", err)
			}
			if len(a.jobs.queued) != 1 {
				t.Fatalf("%d jobs queued, want one", len(a.jobs.queued))
			}
			job := a.jobs.queued[0]
			if job.Kind != queue.KindAiSuggest {
				t.Errorf("job kind %q", job.Kind)
			}
			if job.Payload["prompt"] != testCase.prompt {
				t.Errorf("prompt %v, want %q", job.Payload["prompt"], testCase.prompt)
			}
			if job.Payload["kind"] != string(domain.KindFields) {
				t.Errorf("kind %v", job.Payload["kind"])
			}
			// A proposal unless it is said, which is the milestone's whole shape.
			if _, said := job.Payload["apply"]; said {
				t.Error("the job says apply when nobody asked for it")
			}
		})
	}
}

// "Or applied directly, configured explicitly." The flag has to be *said*, and it reaches the job
// so that the acceptance happens where the answer arrives.
func TestApplyingDirectlyIsSaidRatherThanAssumed(t *testing.T) {
	a := newAsker(true)

	if err := (AiSummarize(a.ask)).Execute(context.Background(), person(), targetID, true); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if a.jobs.queued[0].Payload["apply"] != true {
		t.Errorf("the job does not carry the instruction to apply: %v", a.jobs.queued[0].Payload)
	}
	// And it is in the trail: a workspace where AI writes without anybody reading is a fact about
	// that workspace, and "who configured that" is a question with an answer.
	if len(a.world.entries) != 1 {
		t.Fatalf("%d audit entries", len(a.world.entries))
	}
	recorded := map[string]string{}
	for field, masked := range a.world.entries[0].Changes {
		shape, _ := masked.(map[string]any)
		recorded[field], _ = shape["to"].(string)
	}
	if recorded["applied_directly"] != "true" || recorded["prompt"] != "summarize" {
		t.Errorf("the entry records %v", recorded)
	}
}

// A workspace with no provider is refused before anything is queued, whichever action asked.
func TestEveryActionIsRefusedWithoutAProvider(t *testing.T) {
	a := newAsker(false)

	for _, run := range []func() error{
		func() error {
			return AiSuggestFields(a.ask).Execute(context.Background(), person(), targetID, false)
		},
		func() error {
			return AiSummarize(a.ask).Execute(context.Background(), person(), targetID, false)
		},
		func() error {
			return AiClassify(a.ask).Execute(context.Background(), person(), targetID, false)
		},
	} {
		if err := run(); !errors.Is(err, shared.ErrUnavailable) {
			t.Fatalf("the answer was %v, want an unavailable dependency", err)
		}
	}
	if len(a.jobs.queued) != 0 {
		t.Error("a refused workspace had questions queued for it")
	}
}

// The descriptors are what make these automation actions at all: the kind a rule names is derived
// from the use case name, so AI_SUGGEST_FIELDS exists exactly when AiSuggestFields does.
func TestTheThreeActionsAreNamedAsAutomationExpects(t *testing.T) {
	a := newAsker(true)

	for _, testCase := range []struct {
		descriptor usecase.Descriptor
		action     string
	}{
		{AiSuggestFields(a.ask).Descriptor(), "AI_SUGGEST_FIELDS"},
		{AiSummarize(a.ask).Descriptor(), "AI_SUMMARIZE"},
		{AiClassify(a.ask).Descriptor(), "AI_CLASSIFY"},
	} {
		t.Run(testCase.action, func(t *testing.T) {
			if got := testCase.descriptor.AutomationAction(); got != testCase.action {
				t.Errorf("the action is %q, want %q - automation.md §1.3 names it", got, testCase.action)
			}
			if !testCase.descriptor.Audit.Required {
				t.Error("a writing use case declares no audit obligation")
			}
			var declared []string
			for _, field := range testCase.descriptor.Input {
				declared = append(declared, field.Name)
			}
			if len(declared) != 2 || declared[0] != "item_id" || declared[1] != "apply" {
				t.Errorf("the input is %v", declared)
			}
		})
	}
}

// Through the registry, `apply` absent is a proposal - the default a rule gets by not saying.
func TestThroughTheRegistryApplyDefaultsToProposing(t *testing.T) {
	a := newAsker(true)

	if _, err := (AiClassify(a.ask)).invoke(context.Background(), person(),
		usecase.Input{"item_id": targetID.String()}); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if _, said := a.jobs.queued[0].Payload["apply"]; said {
		t.Error("an action that said nothing about applying applies anyway")
	}
}

// The other two thirds of §2's Summarisation row (K-05): the same audit action, their own prompts,
// and one of them about a collection rather than an entry.
func TestTheTwoSummariesAskTheirOwnQuestionsAboutTheirOwnTargets(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		run          func(asker) error
		prompt       string
		targetType   string
		targetID     shared.ID
		automation   string
		declaredKeys []string
	}{
		{"AI_SUMMARIZE_THREAD", func(a asker) error {
			return AiSummarizeThread(a.ask).Execute(context.Background(), person(), targetID, false)
		}, "summarize-thread", "WORK_ITEM", targetID, "AI_SUMMARIZE_THREAD", []string{"item_id", "apply"}},
		{"AI_SUMMARIZE_CONTAINER", func(a asker) error {
			return AiSummarizeContainer(a.ask).Execute(context.Background(), person(), containerTargetID)
		}, "summarize-collection", "CONTAINER", containerTargetID, "AI_SUMMARIZE_CONTAINER",
			[]string{"container_id"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			a := newAsker(true)

			if err := testCase.run(a); err != nil {
				t.Fatalf("asking: %v", err)
			}
			if len(a.jobs.queued) != 1 {
				t.Fatalf("%d jobs queued", len(a.jobs.queued))
			}
			job := a.jobs.queued[0]
			if job.Payload["prompt"] != testCase.prompt {
				t.Errorf("prompt %v, want %q", job.Payload["prompt"], testCase.prompt)
			}
			if job.Payload["target_type"] != testCase.targetType {
				t.Errorf("target type %v, want %q", job.Payload["target_type"], testCase.targetType)
			}
			if job.Payload["target_id"] != testCase.targetID.String() {
				t.Errorf("target %v", job.Payload["target_id"])
			}
			// One audit action for all three summaries: "somebody asked, about what, with which
			// prompt" is the shape ai.summary_asked already had.
			if len(a.world.entries) != 1 || a.world.entries[0].Action != SummaryAskedAction {
				t.Errorf("the trail is %+v", a.world.entries)
			}

			var descriptor usecase.Descriptor
			switch testCase.targetType {
			case "CONTAINER":
				descriptor = AiSummarizeContainer(a.ask).Descriptor()
			default:
				descriptor = AiSummarizeThread(a.ask).Descriptor()
			}
			if got := descriptor.AutomationAction(); got != testCase.automation {
				t.Errorf("the action is %q, want %q", got, testCase.automation)
			}
			var declared []string
			for _, field := range descriptor.Input {
				declared = append(declared, field.Name)
			}
			if strings.Join(declared, ",") != strings.Join(testCase.declaredKeys, ",") {
				t.Errorf("the input is %v, want %v", declared, testCase.declaredKeys)
			}
		})
	}
}

// A collection summary is asked for through the registry the way a rule would ask for it.
func TestAContainerSummaryIsAskedForByContainer(t *testing.T) {
	a := newAsker(true)

	if _, err := (AiSummarizeContainer(a.ask)).invoke(context.Background(), person(),
		usecase.Input{"container_id": containerTargetID.String()}); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if a.jobs.queued[0].Payload["target_type"] != "CONTAINER" {
		t.Errorf("the job is about %v", a.jobs.queued[0].Payload["target_type"])
	}
}

// A workspace with no provider is refused whichever summary asked, and nothing is queued.
func TestTheSummariesAreRefusedWithoutAProvider(t *testing.T) {
	a := newAsker(false)

	if err := (AiSummarizeThread(a.ask)).
		Execute(context.Background(), person(), targetID, false); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the answer was %v", err)
	}
	if err := (AiSummarizeContainer(a.ask)).
		Execute(context.Background(), person(), containerTargetID); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the answer was %v", err)
	}
	if len(a.jobs.queued) != 0 {
		t.Error("a refused workspace had questions queued for it")
	}
}

// Both are refused for somebody who may not read what they would summarise, and the refusal comes
// from the read rather than from a second permission written beside it: the entry and the
// collection are read through their own use cases, which is where that question is answered.
func TestASummaryIsRefusedForWhatTheAskerMayNotRead(t *testing.T) {
	for _, testCase := range []struct {
		name string
		run  func(asker) error
	}{
		{"an entry", func(a asker) error {
			return AiSummarizeThread(a.ask).Execute(context.Background(), person(), targetID, false)
		}},
		{"a collection", func(a asker) error {
			return AiSummarizeContainer(a.ask).Execute(context.Background(), person(), containerTargetID)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			a := newAsker(true)
			a.world.readFails = shared.ErrNotFound.WithDetail("containers.not_found")

			if err := testCase.run(a); !errors.Is(err, shared.ErrNotFound) {
				t.Fatalf("the answer was %v, want the read's own refusal", err)
			}
			if len(a.jobs.queued) != 0 {
				t.Error("a question was queued about something the asker cannot read")
			}
			if len(a.world.entries) != 0 {
				t.Error("a refused ask was recorded as one that happened")
			}
		})
	}
}

// The fixtures.

type asker struct {
	ask   Ask
	jobs  *jobQueue
	world *world
}

func newAsker(available bool) asker {
	cases, w := newWorld()
	jobs := &jobQueue{}
	return asker{
		ask:   Ask{Cases: cases, AI: availabilityDouble{available: available}, Queue: jobs},
		jobs:  jobs,
		world: w,
	}
}

type jobQueue struct{ queued []queue.Request }

func (q *jobQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.queued = append(q.queued, request)
	return shared.MustParseID("0192f000-0000-7000-8000-00000000ac01"), nil
}

type availabilityDouble struct{ available bool }

func (a availabilityDouble) CanSuggest(context.Context, appshared.ActorContext) (bool, error) {
	return a.available, nil
}
