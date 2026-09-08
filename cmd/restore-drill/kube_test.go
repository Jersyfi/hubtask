// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is enough of an API server for the four requests the drill makes: clusters and
// ConfigMaps in one namespace, keyed by name.
type fakeAPI struct {
	mu         sync.Mutex
	clusters   map[string]map[string]any
	configMaps map[string]map[string]any
	seenToken  string
}

func (f *fakeAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.seenToken = r.Header.Get("Authorization")

		var store map[string]map[string]any
		var prefix string
		switch {
		case strings.HasPrefix(r.URL.Path, "/apis/postgresql.cnpg.io/v1/namespaces/ns/clusters"):
			store, prefix = f.clusters, "/apis/postgresql.cnpg.io/v1/namespaces/ns/clusters"
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/ns/configmaps"):
			store, prefix = f.configMaps, "/api/v1/namespaces/ns/configmaps"
		default:
			http.Error(w, `{"message":"no such path"}`, http.StatusNotFound)
			return
		}
		name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")

		switch r.Method {
		case http.MethodGet:
			object, ok := store[name]
			if !ok {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodPost:
			var object map[string]any
			_ = json.NewDecoder(r.Body).Decode(&object)
			metadata, _ := object["metadata"].(map[string]any)
			created, _ := metadata["name"].(string)
			if _, exists := store[created]; exists {
				http.Error(w, `{"message":"already exists"}`, http.StatusConflict)
				return
			}
			store[created] = object
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodPut:
			var object map[string]any
			_ = json.NewDecoder(r.Body).Decode(&object)
			store[name] = object
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodDelete:
			if _, ok := store[name]; !ok {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			delete(store, name)
			w.WriteHeader(http.StatusOK)
		}
	})
}

func newTestClient(t *testing.T) (*kubeClient, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{clusters: map[string]map[string]any{}, configMaps: map[string]map[string]any{}}
	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := newKubeClient(kubeConfig{
		APIURL:    server.URL,
		TokenFile: token,
		CAFile:    filepath.Join(dir, "absent-ca.crt"),
	}, "ns")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client, api
}

func TestTheClientCreatesReadsAndDeletesAClusterWithItsToken(t *testing.T) {
	client, api := newTestClient(t)
	ctx := context.Background()

	manifest := map[string]any{"metadata": map[string]any{"name": "db-drill"}, "spec": map[string]any{}}
	if err := client.createCluster(ctx, manifest); err != nil {
		t.Fatalf("create: %v", err)
	}
	if api.seenToken != "Bearer secret-token" {
		t.Errorf("the token travelled as %q", api.seenToken)
	}
	if err := client.createCluster(ctx, manifest); err == nil {
		t.Error("creating the same cluster twice was accepted")
	}

	found, object, err := client.getCluster(ctx, "db-drill")
	if err != nil || !found || object["metadata"].(map[string]any)["name"] != "db-drill" {
		t.Fatalf("get: found=%t err=%v", found, err)
	}
	if found, _, err := client.getCluster(ctx, "nothing"); err != nil || found {
		t.Errorf("an absent cluster: found=%t err=%v", found, err)
	}

	if err := client.deleteCluster(ctx, "db-drill"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := client.deleteCluster(ctx, "db-drill"); err != nil {
		t.Errorf("deleting an absent cluster is an error: %v", err)
	}
}

func TestTheRecordIsCreatedThenMergedKeepingOtherKeys(t *testing.T) {
	client, api := newTestClient(t)
	ctx := context.Background()

	if err := client.mergeConfigMap(ctx, "record", map[string]string{"last_run": "one", "last_success": "one"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := client.mergeConfigMap(ctx, "record", map[string]string{"last_run": "two"}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	data := api.configMaps["record"]["data"].(map[string]any)
	if data["last_run"] != "two" || data["last_success"] != "one" {
		t.Errorf("the record holds %v", data)
	}
	if ns := api.configMaps["record"]["metadata"].(map[string]any)["namespace"]; ns != "ns" {
		t.Errorf("the record was created in %v", ns)
	}
}

func TestAnAPIErrorNamesTheServersMessageAndNotTheToken(t *testing.T) {
	err := apiError("creating the temporary cluster", 403, []byte(`{"message":"clusters is forbidden for this account"}`))
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error %v", err)
	}
	if err := apiError("x", 500, nil); !strings.Contains(err.Error(), "Internal Server Error") {
		t.Errorf("error without a body: %v", err)
	}
}

func TestOutsideAPodTheClientSaysSo(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := newKubeClient(kubeConfig{TokenFile: "/nonexistent"}, "ns"); err == nil {
		t.Error("no address and no token was accepted")
	}
}
