// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// mainHostsFake records the base each call names.
type mainHostsFake struct{ bases []string }

func (m *mainHostsFake) EnsureMain(_ context.Context, base string) (int, error) {
	m.bases = append(m.bases, base)
	return 2, nil
}

// The start writes the missing main rows under the installation's host, read as a row holds it, in
// the system scope the door writes under; an installation without a host writes nothing.
func TestTheStartWritesTheMainRowsUnderTheInstallationsHost(t *testing.T) {
	hosts, uow := &mainHostsFake{}, &unitOfWork{}
	written, err := EnsureMainHosts{Hosts: hosts, UnitOfWork: uow, BaseHost: "Hub.Example:8443"}.Apply(t.Context())
	if err != nil || written != 2 {
		t.Fatalf("answered (%d, %v)", written, err)
	}
	if len(hosts.bases) != 1 || hosts.bases[0] != "hub.example" {
		t.Errorf("asked with %v, want the host alone", hosts.bases)
	}
	if len(uow.scopes) != 1 || uow.scopes[0] != persistence.SystemScope() {
		t.Errorf("scopes %v, want the system scope", uow.scopes)
	}

	written, err = EnsureMainHosts{Hosts: hosts, UnitOfWork: uow}.Apply(t.Context())
	if err != nil || written != 0 || len(hosts.bases) != 1 {
		t.Errorf("without a host it answered (%d, %v) and asked %v", written, err, hosts.bases)
	}
}
