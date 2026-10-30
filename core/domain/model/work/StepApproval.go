// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ApprovalRule is how many of a level's approvers must say yes (UC-SVC-08 check 2). One decline
// declines the level whatever the rule, so a no is never outvoted silently.
type ApprovalRule string

const (
	ApprovalAny      ApprovalRule = "ANY"
	ApprovalAll      ApprovalRule = "ALL"
	ApprovalNOf      ApprovalRule = "N_OF"
	ApprovalMajority ApprovalRule = "MAJORITY"
)

// Bounds of an approval: a sequence a process names, far short of a list of everybody.
const (
	MaxApprovalLevels    = 5
	MaxApprovalApprovers = 50
	MaxApprovalDueHours  = 24 * 90
)

// ApprovalLevel is one level of a step's approval: who may decide and how many must say yes. The
// people, groups and person fields are counted when the approval opens; a role is held along
// the entry's path when somebody decides, so a level naming a role asks one or N yeses, never
// all or a majority of a number nobody counted.
type ApprovalLevel struct {
	People       []string
	Groups       []string
	Roles        []string
	PersonFields []string
	Rule         ApprovalRule
	// Count is N of N_OF; zero otherwise.
	Count int
}

// Counted reports whether the level's rule needs the number of its approvers.
func (l ApprovalLevel) Counted() bool { return l.Rule == ApprovalAll || l.Rule == ApprovalMajority }

// Needed answers how many yeses pass the level among the approvers counted when it opened.
func (l ApprovalLevel) Needed(counted int) int {
	switch l.Rule {
	case ApprovalAll:
		return counted
	case ApprovalMajority:
		return counted/2 + 1
	case ApprovalNOf:
		return l.Count
	default:
		return 1
	}
}

// StepApproval makes a step wait for people's yes (UC-SVC-08): taking the step opens an approval
// instead of moving the entry; the step is taken once the last level says yes, and a decline
// moves the entry to DeclineTo, or leaves it where it stands.
type StepApproval struct {
	Levels    []ApprovalLevel
	DeclineTo string
	// DueHours is how long each level has before the approval is overdue; zero is no limit.
	DueHours int
}

func newStepApproval(approval StepApproval, shape WorkflowShape, at string) (StepApproval, error) {
	if len(approval.Levels) == 0 || len(approval.Levels) > MaxApprovalLevels {
		return StepApproval{}, workflowInvalid(at+"/levels", "workflows.approval_levels",
			map[string]string{"maximum": strconv.Itoa(MaxApprovalLevels)})
	}
	if approval.DeclineTo != "" {
		if _, found := shape.Status(approval.DeclineTo); !found {
			return StepApproval{}, workflowInvalid(at+"/decline_to", "workflows.status_unknown", map[string]string{"key": approval.DeclineTo})
		}
	}
	if approval.DueHours < 0 || approval.DueHours > MaxApprovalDueHours {
		return StepApproval{}, workflowInvalid(at+"/due_hours", "workflows.approval_due_hours",
			map[string]string{"maximum": strconv.Itoa(MaxApprovalDueHours)})
	}
	levels := make([]ApprovalLevel, 0, len(approval.Levels))
	for i, level := range approval.Levels {
		levelAt := at + "/levels/" + strconv.Itoa(i)
		who, err := newStepWho(StepWho{Roles: level.Roles, PersonFields: level.PersonFields, Groups: level.Groups}, levelAt)
		if err != nil {
			return StepApproval{}, err
		}
		level.Roles, level.PersonFields, level.Groups = who.Roles, who.PersonFields, who.Groups
		level.People = distinctTrimmed(level.People)
		for _, person := range level.People {
			if id, err := shared.ParseID(person); err != nil || id.IsZero() {
				return StepApproval{}, workflowInvalid(levelAt+"/people", "workflows.approver_unknown", map[string]string{"person": person})
			}
		}
		named := len(level.People) + len(level.Groups) + len(level.PersonFields)
		if named+len(level.Roles) == 0 {
			return StepApproval{}, workflowInvalid(levelAt, "workflows.approval_nobody", nil)
		}
		if named+len(level.Roles) > MaxApprovalApprovers {
			return StepApproval{}, workflowInvalid(levelAt, "workflows.approval_too_many",
				map[string]string{"maximum": strconv.Itoa(MaxApprovalApprovers)})
		}
		switch level.Rule {
		case ApprovalAny, ApprovalAll, ApprovalMajority:
			level.Count = 0
		case ApprovalNOf:
			if level.Count < 1 || level.Count > MaxApprovalApprovers {
				return StepApproval{}, workflowInvalid(levelAt+"/count", "workflows.approval_count", nil)
			}
		default:
			return StepApproval{}, workflowInvalid(levelAt+"/rule", "workflows.approval_rule_unknown", map[string]string{"rule": string(level.Rule)})
		}
		if level.Counted() && len(level.Roles) > 0 {
			return StepApproval{}, workflowInvalid(levelAt+"/roles", "workflows.approval_role_uncounted", nil)
		}
		levels = append(levels, level)
	}
	approval.Levels = levels
	return approval, nil
}

func distinctTrimmed(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}
