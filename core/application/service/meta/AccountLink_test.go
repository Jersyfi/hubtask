// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
)

type accountLinkTold struct{ link string }

func (a accountLinkTold) For(_ context.Context, actor appshared.ActorContext) (string, error) {
	if !actor.IsAuthenticated() {
		return "", nil
	}
	return a.link, nil
}

// The manifest answers the provider's account link as the one decision answers it - to the owners
// and administrators it is for - and nothing where none is told or nobody is signed in.
func TestTheManifestAnswersTheAccountLinkToWhomItIsFor(t *testing.T) {
	h := handler(profiles{list: systemDefaults()}, &unitOfWork{})
	h.AccountLinks = accountLinkTold{link: "https://provider.example/account"}

	told, err := h.Execute(t.Context(), member())
	if err != nil || told.AccountLink != "https://provider.example/account" {
		t.Fatalf("the told caller read %q, %v", told.AccountLink, err)
	}
	anonymous, err := h.Execute(t.Context(), appshared.Anonymous("en", "UTC"))
	if err != nil || anonymous.AccountLink != "" {
		t.Fatalf("an anonymous caller read %q, %v", anonymous.AccountLink, err)
	}
	h.AccountLinks = accountLinkTold{}
	if untold, err := h.Execute(t.Context(), member()); err != nil || untold.AccountLink != "" {
		t.Fatalf("a caller not told read %q, %v", untold.AccountLink, err)
	}
}
