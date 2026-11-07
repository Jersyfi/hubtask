// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func TestAHooksSecretFollowsItsSaltAndItsInstallation(t *testing.T) {
	a := NewCodeHookSecrets(secret.New("installation-one"))
	b := NewCodeHookSecrets(secret.New("installation-two"))
	tenant, hook := shared.ID("t"), shared.ID("h")
	first := a.Derive(tenant, hook, "salt-1")
	if first != a.Derive(tenant, hook, "salt-1") {
		t.Fatal("the same salt derived another secret")
	}
	if first == a.Derive(tenant, hook, "salt-2") || first == b.Derive(tenant, hook, "salt-1") ||
		first == a.Derive(shared.ID("other"), hook, "salt-1") {
		t.Fatal("a secret did not change with its salt, its installation or its workspace")
	}
}
