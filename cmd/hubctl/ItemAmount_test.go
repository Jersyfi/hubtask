// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// An amount travels as the contract's own field (UC-WRK-25 check 2), and an absent flag sends none.
func TestCreatingAnItemWithAnAmountSendsIt(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneItem)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "TASK", "--title", "milk", "--amount", "2")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"amount":"2"`) {
		t.Errorf("the body %q carries no amount", stub.body)
	}

	stub = serveJSON(t, http.StatusCreated, oneItem)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "TASK", "--title", "milk"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(stub.body, `"amount"`) {
		t.Errorf("an amount nobody gave was sent: %q", stub.body)
	}
}
