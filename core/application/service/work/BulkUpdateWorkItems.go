// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	BulkUpdateWorkItemsName = "BulkUpdateWorkItems"

	// ItemBulkAppliedAction is the audit code for the bulk itself: who sent one, how big it was,
	// and how much of it applied.
	//
	// It does not describe the changes - each operation writes its own entry through the use case
	// that performed it, and the request identifier on all of them is what ties them together
	// (audit.md §2). What this entry adds is the act of sending five hundred changes at once, which
	// no per-entry record shows.
	ItemBulkAppliedAction audit.Action = "item.bulk_applied"

	// maxBulkOperations is the cap api-guidelines.md §5 states.
	maxBulkOperations = 500
)

// bulkOperations maps the contract's operation names onto the use cases that perform them
// (api/openapi.yaml, schema BulkOperation).
//
// A table rather than a switch with a body per operation, because that is the whole design: a bulk
// performs the operations this API already offers on one entry, through the same use cases, so what
// a bulk may do is what a caller may do one entry at a time and never more. Nine reimplementations
// would be nine places for a rule to drift from the one the single-entry route enforces.
var bulkOperations = map[string]string{
	"CREATE_ITEM":   CreateWorkItemName,
	"UPDATE_ITEM":   UpdateWorkItemName,
	"COMPLETE_ITEM": CompleteWorkItemName,
	"REOPEN_ITEM":   ReopenWorkItemName,
	"MOVE_ITEM":     MoveWorkItemName,
	"TRASH_ITEM":    TrashWorkItemName,
	"ADD_LABEL":     AddLabelName,
	"REMOVE_LABEL":  RemoveLabelName,
	"ASSIGN":        AssignWorkItemName,
}

// BulkOperationNames are the operations a bulk accepts, in a stable order. The catalogue declares
// them as the field's enum, so a client is told what it may send rather than having to try.
func BulkOperationNames() []string { return slices.Sorted(maps.Keys(bulkOperations)) }

// Catalogue is the slice of the use case registry a bulk needs: run one use case by name.
//
// Declared here as an interface rather than taking the registry, for the reason every other narrow
// port in this package is narrow - and for one more: the registry is built *from* this use case's
// descriptor, so a direct dependency would be a cycle the composition root cannot wire. What is
// passed in is bound after the catalogue exists.
type Catalogue interface {
	Invoke(
		ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
	) (usecase.Output, error)
}

// BulkUpdateWorkItems applies up to five hundred single-entry operations in one call.
//
// Every operation runs through the catalogue, which means it runs exactly as it would on its own:
// its own permission check, its own event, its own entry in the item's history, its own per-field
// records for offline clients and its own metric. One event covering five hundred entries would
// break a merge in a way nothing downstream recovers from, and one history entry for a bulk would
// hide what the history exists to show (api-guidelines.md §5).
//
// What this use case owns is therefore only the two things a bulk adds: the order, and the question
// of what happens when one of them fails.
type BulkUpdateWorkItems struct {
	Catalogue  Catalogue
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// BulkUpdateWorkItemsCommand is the input, typed.
type BulkUpdateWorkItemsCommand struct {
	// Atomic makes the bulk all or nothing. Without it, the operations that succeeded stay applied.
	Atomic     bool
	Operations []BulkOperation
}

// BulkOperation is one operation of a bulk: which single-entry operation, on which entry, with the
// body that operation takes on its own.
type BulkOperation struct {
	Op     string
	ItemID shared.ID
	// Payload is what the single-entry operation takes in its body, by the field names it takes
	// there. It is not inspected here: the catalogue checks it against the operation's own
	// declaration, so a field the operation does not know comes back named rather than ignored.
	Payload map[string]any
}

// BulkOutcome is what one operation did. Exactly one of Output and Err is set, except for an
// operation of an atomic bulk that was rolled back, which carries the rolled-back refusal.
type BulkOutcome struct {
	Index  int
	Op     string
	Output usecase.Output
	Err    *shared.Error
}

// BulkResult is the whole answer: one outcome per operation, in the order they were sent.
type BulkResult struct {
	Outcomes []BulkOutcome
	Applied  int
	Failed   int
}

// Execute applies the operations and reports what each one did.
//
// It returns an error only when the bulk itself could not be carried out - an unreadable request, a
// cap exceeded, a context that ran out. An operation that failed is not that: the bulk was carried
// out, and what happened to each operation is in the answer. A caller reads the outcomes rather
// than one status covering five hundred of them.
func (h BulkUpdateWorkItems) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd BulkUpdateWorkItemsCommand,
) (BulkResult, error) {
	if err := h.check(cmd); err != nil {
		return BulkResult{}, err
	}

	var result BulkResult
	var err error
	if cmd.Atomic {
		result, err = h.atomically(ctx, actor, cmd)
	} else {
		result, err = h.oneByOne(ctx, actor, cmd)
	}
	if err != nil {
		return BulkResult{}, err
	}

	// After the operations, and outside the transaction an atomic bulk ran in, deliberately: a bulk
	// that rolled everything back is exactly the one an auditor wants to see, and an entry written
	// inside that transaction would have been rolled back with it (audit.md §7).
	if err := h.recordAudit(ctx, actor, result); err != nil {
		return BulkResult{}, err
	}
	return result, nil
}

// check refuses the bulk itself: the size, which is the contract's, and nothing about what is in it.
//
// What each operation says is checked where it is performed, so that a bulk of five hundred with
// one bad operation reports one failure rather than being refused whole. The cap is the exception,
// because it is about the request rather than about any operation in it.
func (h BulkUpdateWorkItems) check(cmd BulkUpdateWorkItemsCommand) error {
	switch {
	case len(cmd.Operations) == 0:
		return shared.ErrValidation.
			WithDetail("bulk.no_operations").
			WithFields(shared.FieldError{Path: "/operations", Code: "bulk.no_operations"})
	case len(cmd.Operations) > maxBulkOperations:
		return shared.ErrValidation.
			WithDetail("bulk.too_many_operations").
			WithParams(map[string]string{
				"limit": strconv.Itoa(maxBulkOperations),
				"sent":  strconv.Itoa(len(cmd.Operations)),
			}).
			WithFields(shared.FieldError{Path: "/operations", Code: "bulk.too_many_operations"})
	}
	return nil
}

// oneByOne applies what it can and reports the rest.
//
// Each operation opens its own transaction, through the use case that performs it, so an operation
// that fails takes nothing with it: the ones before it are committed, and the ones after it are
// tried. That is the whole difference from the atomic bulk, and it is expressed by not opening a
// transaction here rather than by unwinding one.
func (h BulkUpdateWorkItems) oneByOne(
	ctx context.Context, actor appshared.ActorContext, cmd BulkUpdateWorkItemsCommand,
) (BulkResult, error) {
	result := BulkResult{Outcomes: make([]BulkOutcome, 0, len(cmd.Operations))}

	for index, operation := range cmd.Operations {
		if err := ctx.Err(); err != nil {
			// The deadline for the request is the bound on how long a bulk may run (rule 7).
			// Reported as the bulk failing rather than as five hundred failed operations: what ran
			// out is the call, not any operation in it.
			return BulkResult{}, shared.ErrUnavailable.
				WithDetail("bulk.deadline_exceeded").
				WithParams(map[string]string{"applied": strconv.Itoa(result.Applied)}).
				WithCause(err)
		}
		result.add(h.apply(ctx, actor, index, operation))
	}
	return result, nil
}

// atomically applies all of them or none of them.
//
// One transaction around the whole bulk: every use case inside it joins the running transaction
// rather than opening a second one (persistence.UnitOfWork), so the first refusal unwinds
// everything the operations before it wrote. The refusal itself is returned from the closure
// because that is what rolls the transaction back - and it is not returned to the caller, because
// the caller asked for a bulk and the bulk was carried out. What happened is in the outcomes.
//
// The cost of the rollback is one an atomic bulk cannot avoid: the audit entry a refused operation
// writes is rolled back with everything else. The entry this use case writes afterwards is what is
// left of it, and it says how many operations applied - which for a rolled-back bulk is none.
func (h BulkUpdateWorkItems) atomically(
	ctx context.Context, actor appshared.ActorContext, cmd BulkUpdateWorkItemsCommand,
) (BulkResult, error) {
	var result BulkResult

	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		result = BulkResult{Outcomes: make([]BulkOutcome, 0, len(cmd.Operations))}

		for index, operation := range cmd.Operations {
			if err := ctx.Err(); err != nil {
				return shared.ErrUnavailable.WithDetail("bulk.deadline_exceeded").WithCause(err)
			}

			outcome := h.apply(ctx, actor, index, operation)
			result.add(outcome)
			if outcome.Err != nil {
				return outcome.Err
			}
		}
		return nil
	})

	switch {
	case err == nil:
		return result, nil
	case len(result.Outcomes) == 0 || result.Outcomes[len(result.Outcomes)-1].Err == nil:
		// The transaction failed for a reason no operation reported - it could not be opened, or
		// the commit itself failed. There is nothing per operation to say about that, so it is the
		// bulk that failed.
		return BulkResult{}, err
	}
	return rolledBack(result, cmd.Operations), nil
}

// rolledBack rewrites the outcomes of an atomic bulk that unwound: everything except the operation
// that refused is reported as not applied, whether it ran and was rolled back or never ran at all.
//
// One code for both, deliberately. The difference is invisible to a client and irrelevant to it:
// nothing was applied either way, and what it has to do is fix the operation that refused and send
// the bulk again.
func rolledBack(result BulkResult, operations []BulkOperation) BulkResult {
	failure := result.Outcomes[len(result.Outcomes)-1]

	unwound := BulkResult{Outcomes: make([]BulkOutcome, 0, len(operations)), Failed: len(operations)}
	for index, operation := range operations {
		if index == failure.Index {
			unwound.Outcomes = append(unwound.Outcomes, failure)
			continue
		}
		unwound.Outcomes = append(unwound.Outcomes, BulkOutcome{
			Index: index, Op: operation.Op,
			Err: shared.ErrConflict.
				WithDetail("bulk.rolled_back").
				WithParams(map[string]string{"failed_index": strconv.Itoa(failure.Index)}),
		})
	}
	return unwound
}

// apply performs one operation through the catalogue.
func (h BulkUpdateWorkItems) apply(
	ctx context.Context, actor appshared.ActorContext, index int, operation BulkOperation,
) BulkOutcome {
	name, known := bulkOperations[operation.Op]
	if !known {
		// The operation the catalogue does not offer is this operation's failure rather than the
		// bulk's: a client that mistyped one of five hundred is told which one.
		return BulkOutcome{Index: index, Op: operation.Op, Err: shared.ErrValidation.
			WithDetail("bulk.operation_unknown").
			WithParams(map[string]string{
				"op":      operation.Op,
				"allowed": strings.Join(BulkOperationNames(), ", "),
			}).
			WithFields(shared.FieldError{
				Path: "/operations/" + strconv.Itoa(index) + "/op", Code: "bulk.operation_unknown",
			})}
	}

	in := usecase.Input{}
	maps.Copy(in, operation.Payload)
	if !operation.ItemID.IsZero() {
		// The operation's own field wins over anything the payload repeats: `item_id` is where the
		// contract puts the entry, and two spellings of it in one operation must not disagree.
		in["item_id"] = operation.ItemID.String()
	}

	out, err := h.Catalogue.Invoke(ctx, name, actor, in)
	if err != nil {
		return BulkOutcome{Index: index, Op: operation.Op, Err: shared.AsError(err)}
	}
	return BulkOutcome{Index: index, Op: operation.Op, Output: out}
}

// add records one outcome and keeps the two counts in step with it.
func (r *BulkResult) add(outcome BulkOutcome) {
	r.Outcomes = append(r.Outcomes, outcome)
	if outcome.Err != nil {
		r.Failed++
		return
	}
	r.Applied++
}

// recordAudit writes the one entry the bulk itself owes: who sent it, how big it was, and how much
// of it applied. All of it is structure - three counts - so all of it is OPEN in the data
// catalogue's sense, and no user content reaches the trail (rule 10).
//
// The target is the tenant rather than an entry, because a bulk is about many of them and picking
// one would be arbitrary; what each operation did to which entry is in the entry that operation
// wrote, under the same request identifier (audit.md §2, EmptyTrash writes its entry the same way).
func (h BulkUpdateWorkItems) recordAudit(
	ctx context.Context, actor appshared.ActorContext, result BulkResult,
) error {
	now := h.Clock.Now()

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     ItemBulkAppliedAction,
			Outcome:    outcomeOf(result),
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: itemTarget,
			TargetID:   actor.TenantID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{
					Field: "operations", Classification: audit.Open,
					To: strconv.Itoa(len(result.Outcomes)),
				},
				audit.Change{
					Field: "applied", Classification: audit.Open, To: strconv.Itoa(result.Applied),
				},
				audit.Change{
					Field: "failed", Classification: audit.Open, To: strconv.Itoa(result.Failed),
				},
			),
		})
	})
}

// outcomeOf is how the trail reads a bulk: a success when everything applied, and a failure when
// anything did not. A bulk that half applied is not a success to an auditor asking whether what
// somebody asked for happened.
func outcomeOf(result BulkResult) audit.Outcome {
	if result.Failed > 0 {
		return audit.OutcomeFailed
	}
	return audit.OutcomeSuccess
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4) - and a bulk is what an agent reaches for when it has
// worked out that fifty entries need the same change, so the MCP door is the point rather than a
// side effect (ai-first.md §1.1).
func (h BulkUpdateWorkItems) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: BulkUpdateWorkItemsName,
		Summary: "Applies up to " + strconv.Itoa(maxBulkOperations) + " single-entry operations in " +
			"one call, in the order they are sent. Each operation is the operation of the same " +
			"name performed on its own, with its own permission check and its own records, so a " +
			"bulk may do exactly what the caller may do one entry at a time. The answer carries a " +
			"result per operation: a bulk that half succeeded is not a failed call, and what " +
			"happened is read per operation. With `atomic`, the first refusal rolls the whole of " +
			"it back and nothing is applied.",
		SideEffects: "Whatever the operations in it do, each with the events, the history entries, " +
			"the offline records and the audit entries it would write on its own, plus one audit " +
			"entry for the bulk itself.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{
				Name: "operations", Kind: usecase.KindList, Required: true,
				Description: "The operations, in the order they are to be applied. Each one is an " +
					"object with `op` (one of " + strings.Join(BulkOperationNames(), ", ") +
					"), `item_id` for every operation except CREATE_ITEM, and `payload` carrying " +
					"what that operation takes in its own request body.",
			},
			{
				Name: "atomic", Kind: usecase.KindBool,
				Description: "All or nothing. Without it a bulk applies what it can and reports " +
					"the rest; with it the first refusal rolls back everything before it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemBulkAppliedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A bulk writes no history of its own. Every operation in it writes the entry " +
				"the use case performing it always writes, on the item it is about - which is " +
				"where a history entry belongs. One entry saying 'a bulk happened' would sit on " +
				"no item at all.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h BulkUpdateWorkItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	operations, err := bulkOperationsFrom(in)
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, BulkUpdateWorkItemsCommand{
		Atomic: in.Bool("atomic"), Operations: operations,
	})
	if err != nil {
		return nil, err
	}
	return bulkOutput(result), nil
}

// bulkOperationsFrom reads the list the catalogue checked the shape of.
//
// The catalogue's declaration reaches as far as "a list arrived"; what is in it is this use case's
// grammar, exactly as a filter tree is the query grammar's (usecase.KindList). What it refuses here
// is the shape - an entry that is not an object, an identifier that is not one - because a value
// that cannot be read at all is not an operation that can be reported as having failed.
func bulkOperationsFrom(in usecase.Input) ([]BulkOperation, error) {
	raw, _ := in["operations"].([]any)

	operations := make([]BulkOperation, 0, len(raw))
	for index, entry := range raw {
		fields, ok := entry.(map[string]any)
		if !ok {
			return nil, bulkShapeError(index, "", "bulk.operation_malformed")
		}

		operation := BulkOperation{}
		if op, held := fields["op"].(string); held {
			operation.Op = op
		} else {
			return nil, bulkShapeError(index, "/op", "bulk.operation_required")
		}

		if identifier, held := fields["item_id"]; held && identifier != nil {
			text, isText := identifier.(string)
			if !isText {
				return nil, bulkShapeError(index, "/item_id", "bulk.item_id_malformed")
			}
			parsed, err := shared.ParseID(text)
			if err != nil {
				return nil, bulkShapeError(index, "/item_id", "bulk.item_id_malformed")
			}
			operation.ItemID = parsed
		}

		if payload, held := fields["payload"]; held && payload != nil {
			document, isObject := payload.(map[string]any)
			if !isObject {
				return nil, bulkShapeError(index, "/payload", "bulk.payload_malformed")
			}
			operation.Payload = document
		}
		operations = append(operations, operation)
	}
	return operations, nil
}

func bulkShapeError(index int, field, code string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithParams(map[string]string{"index": strconv.Itoa(index)}).
		WithFields(shared.FieldError{
			Path: "/operations/" + strconv.Itoa(index) + field, Code: code,
		})
}

// bulkOutput is the shape every channel returns: one result per operation, in the order they were
// sent (api/openapi.yaml, schema BulkResult).
//
// A failed operation carries its refusal as data rather than as an error value, because there is no
// error to return: the call succeeded and this is what happened inside it. The fields are the error
// model's own, so that the REST layer can rebuild the problem it would have answered with - status
// included - and the MCP layer can render the same refusal it renders for a call that failed
// outright.
func bulkOutput(result BulkResult) usecase.Output {
	results := make([]usecase.Output, 0, len(result.Outcomes))
	for _, outcome := range result.Outcomes {
		entry := usecase.Output{"index": outcome.Index, "op": outcome.Op}
		if outcome.Err != nil {
			entry["problem"] = problemOutput(outcome.Err)
		} else {
			entry["output"] = outcome.Output
		}
		results = append(results, entry)
	}
	return usecase.Output{
		"results": results,
		"applied": result.Applied,
		"failed":  result.Failed,
	}
}

// problemOutput renders one refusal as the data every channel builds its own answer from.
func problemOutput(err *shared.Error) usecase.Output {
	problem := usecase.Output{"category": string(err.Category), "code": err.Code}
	if err.DetailCode != "" {
		problem["detail_code"] = err.DetailCode
	}
	if len(err.Params) > 0 {
		problem["params"] = err.Params
	}
	if len(err.Fields) > 0 {
		findings := make([]usecase.Output, 0, len(err.Fields))
		for _, field := range err.Fields {
			finding := usecase.Output{"path": field.Path, "code": field.Code}
			if len(field.Params) > 0 {
				finding["params"] = field.Params
			}
			findings = append(findings, finding)
		}
		problem["field_errors"] = findings
	}
	return problem
}
