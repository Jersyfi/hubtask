// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package cilocal

// Job is one job of `ci.yml` as `make verify-pr` runs it on a laptop.
//
// Filters are the `changes` outputs that start the job on a pull request, any one of them being
// enough; none means the job runs on every pull request. They are a copy of the job's `if:`, and
// test/architecture compares the two, so the copy cannot quietly go stale.
type Job struct {
	ID      string
	Filters []string
	// InVerify says `make verify` already runs what this job runs; verify-pr always starts there.
	InVerify bool
	// Steps run in order, each only when one of its own filters is set (none: the job's).
	Steps []Step
	// Container jobs need Docker and run one session at a time, under a lock: their compose
	// projects and host ports are fixed, and every worktree on a machine shares one daemon.
	Container bool
	// CIOnly is why the job cannot run locally. A job is either run here or says why it is not.
	CIOnly string
}

// Step is one command of a job, run through bash with `.tools` first on the PATH.
type Step struct {
	Name    string
	Filters []string
	Command string
}

// goPipeline is what starts `quick`, and through it every job that needs `quick`.
var goPipeline = []string{"go", "openapi", "db", "deploy", "ci"}

// Jobs lists every job `ci-required` waits for except `changes`, which only classifies, in the
// order verify-pr runs them: the fast and the Go gates first, the workspace next, the containers
// last, because they hold the machine's lock.
var Jobs = []Job{
	{ID: "quick", Filters: goPipeline, InVerify: true},
	{ID: "unit", Filters: goPipeline, InVerify: true},
	{ID: "architecture", Filters: goPipeline, InVerify: true},
	{ID: "security", Filters: goPipeline, InVerify: true},
	{ID: "observability", Filters: goPipeline, InVerify: true},
	{ID: "chart", Filters: goPipeline, InVerify: true},
	{ID: "licences", InVerify: true},
	{ID: "docs", InVerify: true},
	{ID: "build", Filters: goPipeline, Steps: []Step{
		{Name: "build (linux/amd64)", Command: "GOOS=linux GOARCH=amd64 make build"},
		{Name: "build (linux/arm64)", Command: "GOOS=linux GOARCH=arm64 make build"},
	}},
	{ID: "integration", Filters: goPipeline, Steps: []Step{
		{Name: "gate-integration", Command: "make gate-integration"},
		{Name: "gate-contract", Command: "make gate-contract"},
	}},
	{ID: "data", Filters: goPipeline, Steps: []Step{
		{Name: "gate-data", Command: "make gate-data"},
		{Name: "gate-privacy-full", Command: "make gate-privacy-full"},
	}},
	{ID: "resilience", Filters: goPipeline, Steps: []Step{
		{Name: "gate-resilience", Command: "make gate-resilience"},
	}},
	// It edits the working tree while it runs, so nothing else may run beside it - which verify-pr
	// never does, and a session must not do either (gate-selftest.sh).
	{ID: "selftest", Filters: goPipeline, Steps: []Step{
		{Name: "gate-selftest", Command: "make gate-selftest"},
	}},
	{ID: "tokens-drift", Filters: []string{"design_system", "go", "ci"}, Steps: []Step{
		{Name: "tokens without a diff", Command: "make tools-node && pnpm install --frozen-lockfile && make tokens && " +
			"git diff --exit-code -- core/domain/model/shared/LabelTokens.go"},
	}},
	{ID: "node", Filters: []string{"webapp", "website", "design_system", "api_client", "sync_engine", "n8n_node", "zapier_app", "sdk_typescript", "openapi", "ci"}, Steps: []Step{
		{Name: "workspace install and map", Command: "make tools-node && pnpm install --frozen-lockfile && " +
			"node build/lint-workspace-map.mjs --selftest && node build/lint-workspace-map.mjs"},
		pnpmPackage("design-system", "design_system", "ci"),
		pnpmPackage("api-client", "api_client", "openapi", "ci"),
		pnpmPackage("sync-engine", "sync_engine", "api_client", "openapi", "ci"),
		pnpmPackage("n8n-nodes-hubtask", "n8n_node", "api_client", "openapi", "ci"),
		pnpmPackage("zapier-app", "zapier_app", "api_client", "openapi", "ci"),
		pnpmPackage("sdk-typescript", "sdk_typescript", "openapi", "ci"),
		pnpmPackage("webapp", "webapp", "design_system", "sync_engine", "api_client", "openapi", "ci"),
		{Name: "make website", Filters: websiteFilters, Command: "make website"},
		pnpmPackage("website", websiteFilters...),
	}},
	{ID: "engines", Filters: []string{"webapp", "design_system", "sync_engine", "api_client", "openapi", "ci"}, Steps: []Step{
		{Name: "the three engines", Command: "make tools-node && pnpm install --frozen-lockfile && " +
			`pnpm --filter "@hubtask/webapp^..." build && pnpm --filter @hubtask/webapp build && ` +
			"pnpm --filter @hubtask/webapp test:engines"},
	}},
	{ID: "api-client-drift", Filters: []string{"openapi", "api_client", "ci"}, Steps: []Step{
		{Name: "the API client from the contract", Command: "make tools-node && pnpm install --frozen-lockfile && " +
			"make api-client && pnpm -r build && pnpm -r typecheck && git diff --exit-code"},
	}},
	{ID: "compose", Container: true, Filters: []string{"go", "openapi", "db", "deploy", "ci", "webapp", "design_system", "api_client"}, Steps: []Step{
		{Name: "gate-compose", Command: "make gate-compose"},
	}},
	{ID: "e2e", Container: true, Filters: goPipeline, Steps: []Step{
		{Name: "gate-e2e", Command: "make gate-e2e"},
	}},
	{ID: "engine-session", Container: true, Filters: goPipeline, Steps: []Step{
		{Name: "gate-engine-conformance", Command: "make tools-node && pnpm install --frozen-lockfile && make gate-engine-conformance"},
	}},
	{ID: "secrets", CIOnly: "gitleaks scans the history and is not among the pinned tools; push protection stops a key at the push"},
	{ID: "dependencies", CIOnly: "the dependency review compares the pull request with its base through GitHub's API"},
	// Run by verify-pr itself rather than as a step: it needs the description, which lives on GitHub.
	{ID: "pr-description", CIOnly: "the description is read from the pull request - verify-pr checks it with make gate-pr itself"},
}

var websiteFilters = []string{"website", "design_system", "api_client", "openapi", "ci"}

func pnpmPackage(name string, filters ...string) Step {
	filter := `"@hubtask/` + name + `"`
	return Step{
		Name:    "workspace (" + name + ")",
		Filters: filters,
		Command: `pnpm --filter "@hubtask/` + name + `^..." build && pnpm --filter ` + filter + ` build && ` +
			`pnpm --filter ` + filter + ` lint && pnpm --filter ` + filter + ` typecheck && pnpm --filter ` + filter + ` test`,
	}
}

// Runs answers whether a job, or a step of it, starts for the filters a change sets. No filters
// means always.
func Runs(filters []string, touched map[string]bool) bool {
	if len(filters) == 0 {
		return true
	}
	for _, name := range filters {
		if touched[name] {
			return true
		}
	}
	return false
}
