// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/presentation/mcp"
)

// doorsAlike names, per use case check that says "REST, MCP and hubctl ... alike", the operations
// it names. Alike means one use case behind the three doors: REST serves the operation the
// catalogue's use case answers, MCP offers that use case as its tool, and hubctl's generated
// command calls that very operation.
var doorsAlike = []struct {
	check      string
	operations []string
}{
	// UC-AGT-01 check 8: register, read, approve, put live, stop and retire agents.
	{"UC-AGT-01/8", []string{"registerAgent", "listAgents", "readAgent", "approveAgent", "activateAgent", "suspendAgent", "retireAgent"}},
	// UC-AGT-03 check 6: register, list, change and retire models - retiring is a change.
	{"UC-AGT-03/6", []string{"registerAgentModel", "listAgentModels", "changeAgentModel"}},
	// UC-AGT-05 check 4: an entry's provenance.
	{"UC-AGT-05/4", []string{"listValueProvenance"}},
	// UC-AGT-07 check 6: start a job and list an agent's jobs.
	{"UC-AGT-07/6", []string{"startAgentJob", "listAgentJobs"}},
	// UC-GOL-01 check 8: set, read, link and list goals.
	{"UC-GOL-01/8", []string{"createGoal", "updateGoal", "readGoal", "linkGoalEntries", "unlinkGoalEntry", "listGoals", "listEntryGoals"}},
	// UC-GOL-02 check 5: check in and list check-ins.
	{"UC-GOL-02/5", []string{"checkInGoal", "listGoalCheckIns"}},
	// UC-GOL-03 check 7: portfolios and their updates.
	{"UC-GOL-03/7", []string{"createPortfolio", "readPortfolio", "updatePortfolio", "deletePortfolio", "listPortfolios", "reportPortfolioStatus", "listPortfolioUpdates"}},
	// UC-GOL-04 check 7: dashboards and their figures.
	{"UC-GOL-04/7", []string{"createDashboard", "readDashboard", "updateDashboard", "deleteDashboard", "listDashboards", "readDashboardFigures"}},
	// UC-PLN-02 check 8: read the plan, apply it, a dependency's timing and an entry's mode.
	{"UC-PLN-02/8", []string{"readPlanSchedule", "applyPlanSchedule", "setDependencyTiming", "setEntryScheduling"}},
	// UC-PLN-06 check 8: plan, change, start, close and read sprints.
	{"UC-PLN-06/8", []string{"createSprint", "updateSprint", "addSprintEntries", "removeSprintEntries", "startSprint", "closeSprint", "readSprint", "listSprints"}},
	// UC-PLN-07 check 6: an estimate, the burndown (a sprint's read) and the velocity.
	{"UC-PLN-07/6", []string{"setEntryEstimate", "clearEntryEstimate", "readSprint", "readVelocity"}},
	// UC-PLN-08 check 9: plan, change, fill, release and read releases.
	{"UC-PLN-08/9", []string{"createRelease", "updateRelease", "addReleaseEntries", "removeReleaseEntries", "publishRelease", "readRelease", "listReleases"}},
	// UC-PLN-09 check 7: bring a file in.
	{"UC-PLN-09/7", []string{"importPlan"}},
	// UC-TIM-01 check 7: record, change, remove and list time; the timer.
	{"UC-TIM-01/7", []string{"logTime", "updateTimeRecord", "deleteTimeRecord", "listTimeRecords", "startTimer", "stopTimer", "readTimer", "discardTimer"}},
	// UC-TIM-02 check 7: read, copy, hand in and take back a week.
	{"UC-TIM-02/7", []string{"readTimesheet", "copyTimesheetWeek", "submitTimesheet", "withdrawTimesheet"}},
	// UC-TIM-03 check 6: list, read, approve and send back weeks; the policy.
	{"UC-TIM-03/6", []string{"listTimesheets", "readTimesheet", "approveTimesheet", "rejectTimesheet", "readTimePolicy", "setTimePolicy"}},
	// UC-TIM-04 check 5: a report and a plan's actuals.
	{"UC-TIM-04/5", []string{"readTimeReport", "readEstimatesVsActuals"}},
	// UC-TIM-05 check 4: the export.
	{"UC-TIM-05/4", []string{"exportTimeRecords"}},
	// UC-TIM-06 check 6: the workload and a capacity.
	{"UC-TIM-06/6", []string{"readWorkload", "setCapacity", "listCapacities"}},
	// UC-TIM-07 check 6: record and read absences.
	{"UC-TIM-07/6", []string{"recordAbsence", "listAbsences"}},
	// UC-TIM-08 check 7: rates, budgets and the burn.
	{"UC-TIM-08/7", []string{"listBillingRates", "setBillingRate", "changeBillingRate", "listCostRates", "setCostRate", "changeCostRate", "readBudget", "setBudget"}},
}

func TestTheDoorsServeTheseUseCasesAlike(t *testing.T) {
	registry := useCaseCatalogue(t)
	served := servedRoutes(t)
	tools := toolNames(mcp.ToolsOf(registry.All()))
	byOperation := map[string]string{}
	for _, d := range registry.All() {
		byOperation[d.RESTOperation()] = d.MCPTool()
	}
	cli := cliOperations(t)
	generated := string(readFile(t, "../../cmd/hubctl/Doors.gen.go"))

	for _, c := range doorsAlike {
		t.Run(c.check, func(t *testing.T) {
			for _, op := range c.operations {
				useCase := op
				if named := cli[op].useCase; named != "" {
					useCase = strings.ToLower(named[:1]) + named[1:]
				}
				tool, known := byOperation[useCase]
				if !known {
					t.Errorf("%s: no use case of the catalogue answers %s", c.check, op)
					continue
				}
				if !served[op] {
					t.Errorf("%s: REST does not serve %s", c.check, op)
				}
				if !tools[tool] {
					t.Errorf("%s: MCP offers no tool %s for %s", c.check, tool, op)
				}
				if cli[op].cli == "" {
					t.Errorf("%s: api/openapi.yaml gives %s no hubctl command", c.check, op)
				}
				if !regexp.MustCompile(`OperationID: "` + op + `"`).MatchString(generated) {
					t.Errorf("%s: hubctl has no generated command calling %s", c.check, op)
				}
			}
		})
	}
}

// doorOperation is what api/openapi.yaml says of an operation: its hubctl command, and the use
// case it calls where that is not the one of its own name.
type doorOperation struct{ cli, useCase string }

// cliOperations answers the operations of api/openapi.yaml.
func cliOperations(t *testing.T) map[string]doorOperation {
	t.Helper()
	var spec struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(readFile(t, "../../api/openapi.yaml"), &spec); err != nil {
		t.Fatalf("api/openapi.yaml: %v", err)
	}
	out := map[string]doorOperation{}
	for _, operations := range spec.Paths {
		for _, node := range operations {
			if node.Kind != yaml.MappingNode {
				continue // a path's own parameters
			}
			var op struct {
				OperationID string `yaml:"operationId"`
				CLI         string `yaml:"x-hubtask-cli"`
				UseCase     string `yaml:"x-hubtask-rest-use-case"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("api/openapi.yaml: %v", err)
			}
			if op.OperationID != "" {
				out[op.OperationID] = doorOperation{cli: op.CLI, useCase: op.UseCase}
			}
		}
	}
	return out
}
