// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The census's stores are a closed set written in several places: the constants the census and the
// re-sealers label with, the contract's enum the key screen is answered in, and the messages that
// name each kind to the operator. A store missing from the enum is an answer the contract forbids;
// one missing from a message reads as "stored secret" where the operator needed to know which.
func TestEveryCensusStoreIsInTheContractAndNamedByTheMessages(t *testing.T) {
	raw, err := os.ReadFile("../../locales/en.json")
	if err != nil {
		t.Fatalf("reading the catalogue: %v", err)
	}
	var catalogue map[string]string
	if err := json.Unmarshal(raw, &catalogue); err != nil {
		t.Fatalf("decoding the catalogue: %v", err)
	}
	for _, store := range sealingrepo.Stores {
		if !openapi.EncryptionStoreCountStore(store).Valid() {
			t.Errorf("the contract's EncryptionStoreCount.store does not list %q", store)
		}
		for _, code := range []string{"crypto.key_missing", "app.instance.encryption_stranded_kind"} {
			if !strings.Contains(catalogue[code], " "+store+" {") {
				t.Errorf("%s names no %q in its store select", code, store)
			}
		}
	}
}
