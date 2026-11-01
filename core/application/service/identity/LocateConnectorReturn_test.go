// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// awaited is a connector's waiting connections by workspace, read in the workspace the unit of
// work was last opened for - as the real query reads only the transaction's own.
type awaited struct {
	work  *unitOfWork
	until map[shared.ID]map[string]time.Time
	fail  error
}

func (a *awaited) Live(_ context.Context, hash []byte, now time.Time) (bool, error) {
	if a.fail != nil {
		return false, a.fail
	}
	scope := a.work.scopes[len(a.work.scopes)-1]
	expires, held := a.until[scope.TenantID][string(hash)]
	return held && now.Before(expires), nil
}

func (a *awaited) wait(tenant shared.ID, state string, until time.Time) {
	hash := sha256.Sum256([]byte(state))
	if a.until[tenant] == nil {
		a.until[tenant] = map[string]time.Time{}
	}
	a.until[tenant][string(hash[:])] = until
}

var connectorAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func connectorLocator(multi bool) (LocateConnectorReturn, *awaited, *unitOfWork) {
	work := &unitOfWork{}
	mail := &awaited{work: work, until: map[shared.ID]map[string]time.Time{}}
	return LocateConnectorReturn{
		Awaited:    map[string]AwaitedReturns{ConnectorMailbox: mail, ConnectorCalendarPush: &awaited{work: work}},
		UnitOfWork: work, Clock: clock.Fixed(connectorAt),
		Address: WorkspaceAddress{
			Workspaces: &workspaceStore{row: domain.Workspace{Slug: "anna"}},
			BaseURL:    "https://hub.example", Multi: multi,
		},
	}, mail, work
}

// A mailbox's return that lands on the installation's host is located at the workspace's own
// address, read in the workspace the state names.
func TestAWaitingConnectionIsLocatedAtItsWorkspacesAddress(t *testing.T) {
	locate, mail, work := connectorLocator(true)
	state := shared.ReturnState(tenant, "aGFuZGxl")
	mail.wait(tenant, state, connectorAt.Add(time.Minute))

	base, err := locate.Execute(t.Context(), state, ConnectorMailbox)
	if err != nil || base != "https://anna.hub.example" {
		t.Fatalf("(%q, %v), want the workspace's own address", base, err)
	}
	if len(work.scopes) != 1 || work.scopes[0] != (persistence.Scope{TenantID: tenant}) {
		t.Errorf("scopes %v, want the one workspace the state names", work.scopes)
	}

	locate.Address.Multi = false
	if base, _ := locate.Execute(t.Context(), state, ConnectorMailbox); base != "https://hub.example" {
		t.Errorf("single-tenant answer %q, want the installation's address", base)
	}
}

// Unknown, expired, of another kind, of another workspace, malformed or of no known kind: one
// refusal, naming no workspace.
func TestEveryStateButAWaitingOneIsTheSameRefusal(t *testing.T) {
	locate, mail, _ := connectorLocator(true)
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000f2")
	live := shared.ReturnState(tenant, "bGl2ZQ")
	mail.wait(tenant, live, connectorAt.Add(time.Minute))
	expired := shared.ReturnState(tenant, "ZXhwaXJlZA")
	mail.wait(tenant, expired, connectorAt)
	// The workspace part is presented, not proven: a live handle under another workspace's name
	// is not found there.
	elsewhere := shared.ReturnState(other, "bGl2ZQ")

	for name, c := range map[string]struct{ state, kind string }{
		"unknown":        {shared.ReturnState(tenant, "dW5rbm93bg"), ConnectorMailbox},
		"expired":        {expired, ConnectorMailbox},
		"another kind":   {live, ConnectorCalendarPush},
		"no such kind":   {live, "SOMETHING"},
		"named wrongly":  {elsewhere, ConnectorMailbox},
		"malformed":      {"bGl2ZQ", ConnectorMailbox},
		"no file stores": {live, ConnectorFileStore},
	} {
		t.Run(name, func(t *testing.T) {
			base, err := locate.Execute(t.Context(), c.state, c.kind)
			got := shared.AsError(err)
			if base != "" || got == nil || !errors.Is(err, shared.ErrNotFound) || got.DetailCode != "auth.connector_return_not_found" {
				t.Errorf("(%q, %v), want the one refusal", base, err)
			}
		})
	}
}

// A store that cannot be read is a fault, not a refusal that would send the browser on.
func TestAnUnreadableStoreIsNotARefusal(t *testing.T) {
	locate, mail, _ := connectorLocator(true)
	mail.fail = shared.ErrUnavailable.WithDetail("mail.oauth_unavailable")
	_, err := locate.Execute(t.Context(), shared.ReturnState(tenant, "eA"), ConnectorMailbox)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("err %v, want the store's fault", err)
	}
}
