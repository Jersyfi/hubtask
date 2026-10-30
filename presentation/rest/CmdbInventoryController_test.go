// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
)

// The public inventory route. What is decided out here: a post without any credential is
// challenged for basic authentication whatever it names, a basic password reaches the intake as
// the presented secret, and a contact is answered with the inventory task.

type inventoryDeliverer struct {
	receipt cmdbservice.InventoryReceipt
	got     cmdbservice.AlertDelivery
	invoked bool
}

func (d *inventoryDeliverer) Deliver(_ context.Context, delivery cmdbservice.AlertDelivery) (cmdbservice.InventoryReceipt, error) {
	d.got, d.invoked = delivery, true
	return d.receipt, nil
}

func postInventory(t *testing.T, door CmdbInventoryDeliverer, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.Intake = &IntakeDoor{CmdbInventory: door}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/cmdb/inventory-intake/0192f000-0000-7000-8000-000000000001/0192f000-0000-7000-8000-000000000002",
		strings.NewReader(`{"action":"contact"}`))
	request.Header.Set("Content-Type", "application/x-compress-zlib")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestAnInventoryPostWithoutACredentialIsChallengedForBasicAuthentication(t *testing.T) {
	door := &inventoryDeliverer{}
	response := postInventory(t, door, "")
	if response.Code != http.StatusUnauthorized || !strings.HasPrefix(response.Header().Get("WWW-Authenticate"), "Basic ") || door.invoked {
		t.Fatalf("answered %d %q, intake asked: %v", response.Code, response.Header().Get("WWW-Authenticate"), door.invoked)
	}
}

func TestAnInventoryPostsBasicPasswordIsThePresentedSecretAndAContactGetsItsTask(t *testing.T) {
	door := &inventoryDeliverer{receipt: cmdbservice.InventoryReceipt{Contact: true}}
	// "agent:s3cret" in base64.
	response := postInventory(t, door, "Basic YWdlbnQ6czNjcmV0")
	if response.Code != http.StatusOK || door.got.Bearer != "s3cret" || door.got.TenantID != "0192f000-0000-7000-8000-000000000001" {
		t.Fatalf("answered %d: %s, presented %q", response.Code, response.Body, door.got.Bearer)
	}
	var answered struct {
		Status     string         `json:"status"`
		Expiration int            `json:"expiration"`
		Tasks      map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil || answered.Status != "ok" || answered.Expiration != 24 || answered.Tasks["inventory"] == nil {
		t.Fatalf("the agent reads its task: %s", response.Body)
	}
}
