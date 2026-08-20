// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// useCaseCatalogue is the list this gate checks: every use case, exactly as the composition root
// registers it.
//
// Descriptor() reads none of a use case's dependencies, so the zero value is enough to ask what
// it is called, what it declares and what it records - which is what lets this gate run without a
// database, an HTTP server, or a wiring fixture.
//
// The list is maintained by hand, and two tests keep it honest:
// TestEveryUseCaseIsInTheCatalogue finds a use case that is missing here, and
// TestTheCompositionRootRegistersEveryUseCase finds one that is missing in cmd/server.
func useCaseCatalogue(t *testing.T) *usecase.Registry {
	t.Helper()

	// Every use case the composition root registers, in the same order. The list is here rather
	// than derived, because deriving it would mean this gate could not notice a use case that was
	// written and never registered - which is exactly what it is for.
	registry, err := usecase.NewRegistry(nil,
		work.CreateContainer{}.Descriptor(),
		work.CreateWorkItem{}.Descriptor(),
		work.UpdateWorkItem{}.Descriptor(),
		work.RenameContainer{}.Descriptor(),
		work.UpdateContainerPolicies{}.Descriptor(),
		work.ArchiveContainer{}.Descriptor(),
		work.UnarchiveContainer{}.Descriptor(),
		work.MoveContainer{}.Descriptor(),
		work.TrashContainer{}.Descriptor(),
		work.RestoreContainer{}.Descriptor(),
		work.CreateBucket{}.Descriptor(),
		work.ListBuckets{}.Descriptor(),
		work.UpdateBucket{}.Descriptor(),
		work.ReorderBucket{}.Descriptor(),
		work.DeleteBucket{}.Descriptor(),
		work.CreateLabel{}.Descriptor(),
		work.ListLabels{}.Descriptor(),
		work.UpdateLabel{}.Descriptor(),
		work.DeleteLabel{}.Descriptor(),
		work.AddLabel{}.Descriptor(),
		work.RemoveLabel{}.Descriptor(),
		work.GetContainer{}.Descriptor(),
		work.ListContainers{}.Descriptor(),
		work.GetWorkItem{}.Descriptor(),
		work.ListWorkItems{}.Descriptor(),
		work.CompleteWorkItem{}.Descriptor(),
		work.ReopenWorkItem{}.Descriptor(),
		work.MoveWorkItem{}.Descriptor(),
		work.ReorderWorkItem{}.Descriptor(),
		work.ArchiveWorkItem{}.Descriptor(),
		work.UnarchiveWorkItem{}.Descriptor(),
		work.TrashWorkItem{}.Descriptor(),
		work.RestoreWorkItem{}.Descriptor(),
		work.ListTrash{}.Descriptor(),
		lifecycle.PurgeWorkItem{}.Descriptor(),
		lifecycle.EmptyTrash{}.Descriptor(),
		identity.InviteAccount{}.Descriptor(),
		identity.UpdateAccountPreferences{}.Descriptor(),
		identity.GrantMembership{}.Descriptor(),
		identity.RevokeMembership{}.Descriptor(),
		identity.CreateGroup{}.Descriptor(),
		identity.UpdateGroup{}.Descriptor(),
		identity.DeleteGroup{}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("the catalogue is not buildable: %v", err)
	}
	return registry
}

// The acceptance criterion of A-07, and the rule arc42 §4 states: a use case is reachable through
// REST, through MCP and as an automation action - all three, or the build is red.
func TestEveryUseCaseIsReachableThroughEveryChannel(t *testing.T) {
	registry := useCaseCatalogue(t)
	served := servedRoutes(t)
	tools := toolNames(mcp.ToolsOf(registry.All()))
	actions := automation.NewActionDispatcher(registry).Actions()

	for _, descriptor := range registry.All() {
		t.Run(descriptor.Name, func(t *testing.T) {
			if !served[descriptor.RESTOperation()] {
				t.Errorf("%s is not served over REST: api/openapi.yaml declares no operation %q, "+
					"or the controller does not implement it",
					descriptor.Name, descriptor.RESTOperation())
			}
			if !tools[descriptor.MCPTool()] {
				t.Errorf("%s is not reachable as the MCP tool %q", descriptor.Name, descriptor.MCPTool())
			}
			if !contains(actions, descriptor.AutomationAction()) {
				t.Errorf("%s is not available as the automation action %q",
					descriptor.Name, descriptor.AutomationAction())
			}
		})
	}
}

// servedRoutes asks the REST controller itself rather than the specification: an operation the
// document declares and the controller leaves to the pending set is a route that answers 404, and
// "declared" is not the same as "reachable".
func servedRoutes(t *testing.T) map[string]bool {
	t.Helper()

	controller := rest.NewRestController()
	controller.UseCases = stubCatalogue{}
	routes := controller.Routes()

	served := map[string]bool{}
	for _, template := range routes.Routes() {
		method, path, found := strings.Cut(template, " ")
		if !found {
			continue
		}

		// A request that reaches the pending set answers 404 with a detail code saying so. Every
		// operation is probed with an empty body, which is enough: an implemented operation
		// answers something else, whatever it thinks of the body.
		//
		// The path wildcards are filled with a real identifier rather than probed as `{containerId}`.
		// The generated wrapper binds path parameters before it dispatches, so a literal brace fails
		// to parse as a uuid and the operation answers 400 - which is not the pending answer, and the
		// route was therefore counted as served whether anything served it or not. Every
		// parameterised operation passed this gate for free until this line.
		request := httptest.NewRequestWithContext(
			actorContext(t), method, boundPath(path), strings.NewReader("{}"))
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, request)

		if !isPending(recorder) {
			served[operationOf(t, template)] = true
		}
	}
	return served
}

// operationOf reads the operationId api/openapi.yaml gives a route template. The identifiers are
// the specification's, so this gate compares the catalogue against the contract rather than
// against a naming convention repeated in a test.
func operationOf(t *testing.T, template string) string {
	t.Helper()

	method, path, _ := strings.Cut(template, " ")
	path = strings.TrimPrefix(path, rest.APIBasePath)

	spec := string(readFile(t, "../../api/openapi.yaml"))
	// The document is read as text on purpose: a YAML decode would need the schema types the
	// contract package owns, and this gate must not depend on a build tag it does not carry.
	marker := "\n  " + path + ":\n"
	start := strings.Index(spec, marker)
	if start < 0 {
		return ""
	}
	// The leading newline is put back, because the marker above consumed it. Without it the
	// method search below only ever finds a method that is *not* the first one in the path item -
	// which is why this read worked for /containers, whose first method is `get`, and silently
	// returned nothing for every path whose first method is the one being looked for.
	item := "\n" + spec[start+len(marker):]
	if end := strings.Index(item, "\n  /"); end >= 0 {
		item = item[:end]
	}

	methodMarker := "\n    " + strings.ToLower(method) + ":\n"
	start = strings.Index(item, methodMarker)
	if start < 0 {
		return ""
	}
	for _, line := range strings.Split(item[start:], "\n") {
		if id, found := strings.CutPrefix(strings.TrimSpace(line), "operationId: "); found {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// boundPath substitutes a syntactically valid identifier for every wildcard of a route template, so
// that the probe reaches the handler rather than the parameter binding.
func boundPath(template string) string {
	var out strings.Builder

	for {
		start := strings.IndexByte(template, '{')
		if start < 0 {
			out.WriteString(template)
			return out.String()
		}
		end := strings.IndexByte(template[start:], '}')
		if end < 0 {
			out.WriteString(template)
			return out.String()
		}
		out.WriteString(template[:start])
		out.WriteString(probeIdentifier)
		template = template[start+end+1:]
	}
}

// probeIdentifier is a well-formed UUIDv7. Every path parameter of this contract is one
// (api/openapi.yaml, components.parameters), so one value covers them all.
const probeIdentifier = "0192f000-0000-7000-8000-00000000000b"

func isPending(recorder *httptest.ResponseRecorder) bool {
	if recorder.Code != http.StatusNotFound {
		return false
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		return false
	}
	return problem["detail_code"] == "route.operation_not_available"
}

// stubCatalogue answers every invocation, so that an implemented operation is recognisable by not
// being the pending answer. What it returns does not matter: this gate asks whether a route is
// wired, not what it produces.
type stubCatalogue struct{}

func (stubCatalogue) Invoke(context.Context, string, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
	return usecase.Output{"id": "0192f000-0000-7000-8000-00000000000b", "version": 1}, nil
}

func actorContext(t *testing.T) context.Context {
	t.Helper()
	return appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
}

func toolNames(tools []mcp.Tool) map[string]bool {
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	return names
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// Gate SG-13: a use case with security or privacy relevance carries an audit declaration
// (audit.md §7). The registry refuses an incomplete one at startup; this is the half that says
// which use cases have to declare one at all.
func TestEveryWritingUseCaseDeclaresItsAuditAction(t *testing.T) {
	for _, descriptor := range useCaseCatalogue(t).All() {
		if descriptor.ReadOnly {
			continue
		}
		if !descriptor.Audit.Required {
			t.Errorf("%s writes and declares no audit obligation (gate SG-13)", descriptor.Name)
			continue
		}
		if descriptor.Audit.Action == "" || descriptor.Audit.TargetType == "" {
			t.Errorf("%s declares an audit obligation without an action or a target", descriptor.Name)
		}
	}
}

// A use case that exists and is not in the catalogue is reachable through nothing at all - which
// is the failure the catalogue exists to prevent, so it is found here rather than in review.
func TestEveryUseCaseIsInTheCatalogue(t *testing.T) {
	registered := map[string]bool{}
	for _, descriptor := range useCaseCatalogue(t).All() {
		registered[descriptor.Name] = true
	}

	for _, name := range useCaseTypes(t) {
		if !registered[name] {
			t.Errorf("%s declares a Descriptor() and is not in the catalogue this gate checks", name)
		}
	}
}

// And one the composition root does not register is one no running installation serves.
func TestTheCompositionRootRegistersEveryUseCase(t *testing.T) {
	main := string(readFile(t, "../../cmd/server/main.go"))

	for _, name := range useCaseTypes(t) {
		if !strings.Contains(main, name+"{") {
			t.Errorf("cmd/server does not register %s - it exists and no installation serves it", name)
		}
	}
}

// useCaseTypes finds every application service that offers itself to the catalogue, by looking
// for the method that makes it registrable.
func useCaseTypes(t *testing.T) []string {
	t.Helper()
	var names []string

	forEachGoFile(t, []string{"../../core/application/service"}, func(path string, f *ast.File, _ *token.FileSet) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, declaration := range f.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "Descriptor" || function.Recv == nil {
				continue
			}
			if receiver := receiverName(function); receiver != "" {
				names = append(names, receiver)
			}
		}
	})
	return names
}

func receiverName(function *ast.FuncDecl) string {
	if len(function.Recv.List) == 0 {
		return ""
	}
	switch expr := function.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s is not readable: %v", path, err)
	}
	return content
}
