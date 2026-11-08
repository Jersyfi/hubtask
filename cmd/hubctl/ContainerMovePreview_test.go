// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// UC-WRK-39 check 5: hubctl offers the move pre-check the dialog asks, with the body `:move` takes.
func TestTheMovePreviewReachesItsOperation(t *testing.T) {
	const collection = "01936f2a-7c1e-7000-8000-0000000000c1"
	const hub = "01936f2a-7c1e-7000-8000-0000000000b2"
	stub := serveJSON(t, http.StatusOK, `{"movable":false,"obstacles":[{"code":"containers.name_taken","params":{}}],"consequences":[]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "move", "preview", collection, "--target-parent-id", hub)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost || stub.request.URL.Path != APIPath+"/containers/"+collection+":move-preview" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if !strings.Contains(stub.body, `"target_parent_id":"`+hub+`"`) {
		t.Errorf("the body %q does not name the destination", stub.body)
	}
	if !strings.Contains(out, "containers.name_taken") {
		t.Errorf("the obstacle is not shown: %q", out)
	}
}
