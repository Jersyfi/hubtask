// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// byNameCatalogue answers each use case with its own output, so that a walk of several
// operations - a page, then a creation, then a read - meets the shape each one declares.
type byNameCatalogue struct{ outputs map[string]catalogue.Output }

func (c byNameCatalogue) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, _ catalogue.Input,
) (catalogue.Output, error) {
	if out, ok := c.outputs[name]; ok {
		return out, nil
	}
	return nil, shared.ErrNotFound
}

// The Python SDK's example against the in-process server (P-03): the generated client, the
// example's walk and the router the same specification generated, driven by whatever python3
// the runner has. Skipped by name where there is none - a runner without Python is not a
// failure of the client, and the generator's own tests hold the Python source without one.
func TestThePythonSdkExampleWalksTheInProcessServer(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 on the path; the Python SDK is not driven here")
	}

	controller := rest.NewRestController()
	controller.UseCases = byNameCatalogue{outputs: map[string]catalogue.Output{
		"ListContainers": {
			"data": []catalogue.Output{containerProjection()},
			"page": map[string]any{"next_cursor": nil, "has_more": false},
		},
		"CreateWorkItem": itemProjection(),
		"GetWorkItem":    itemProjection(),
	}}
	routes := controller.Routes()
	actor := appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hbt_pat_test" {
			t.Errorf("the Python SDK sent %q", r.Header.Get("Authorization"))
		}
		routes.ServeHTTP(w, r.WithContext(appshared.ContextWithActor(r.Context(), actor)))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	example := filepath.Join("..", "..", "sdk", "python", "examples", "quickstart.py")
	cmd := exec.CommandContext(ctx, python, example)
	cmd.Env = append(os.Environ(),
		"HUBTASK_URL="+server.URL+rest.APIBasePath,
		"HUBTASK_TOKEN=hbt_pat_test",
		"HUBTASK_HUB=0192f000-0000-7000-8000-00000000000b",
		"PYTHONDONTWRITEBYTECODE=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the example failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Private", "created 0192f000-0000-7000-8000-00000000000e", "read "} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the example did not print %q:\n%s", want, out)
		}
	}
}
