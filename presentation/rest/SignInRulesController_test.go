// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-INS-22 check 8: the card's choice and the pictures it draws reach the contract's document; a
// card that shows the product's name carries neither.
func TestTheSignInRulesCarryTheCardsLookToTheDocument(t *testing.T) {
	symbol := "0192f000-0000-7000-8000-000000000603"
	shown := signInRulesResponse(usecase.Output{
		"workspace_name":     "Choir",
		"workspace_header":   "WORKSPACE_NAME",
		"workspace_pictures": usecase.Output{"symbol_media_id": symbol},
	})
	if shown.WorkspaceHeader == nil || string(*shown.WorkspaceHeader) != "WORKSPACE_NAME" {
		t.Fatalf("the header reached the document as %v", shown.WorkspaceHeader)
	}
	pictures := shown.WorkspacePictures
	if pictures == nil || pictures.SymbolMediaId == nil || pictures.SymbolMediaId.String() != symbol ||
		pictures.LogoLightMediaId != nil || pictures.LogoDarkMediaId != nil {
		t.Errorf("the pictures reached the document as %+v", pictures)
	}

	product := signInRulesResponse(usecase.Output{"workspace_name": "Choir"})
	if product.WorkspaceHeader != nil || product.WorkspacePictures != nil {
		t.Errorf("the product's name carries %v and %v", product.WorkspaceHeader, product.WorkspacePictures)
	}
}
