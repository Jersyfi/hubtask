// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package httpclient

import (
	"context"
	"crypto/tls"
	"net"
	"syscall"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// codeDialerUnwired is the refusal of a dialler built without a guard: an adapter that forgot
// the wiring must fail closed, not dial unguarded.
const codeDialerUnwired = "dependency.guard_unwired"

// GuardedDialer is GuardedClient's address policy for a protocol that is not HTTP - IMAP, LDAP,
// SSH, clamd. It runs the three checks in one order every adapter shares (rule 6, T-07): the
// egress allowlist, resolution, and the address at dial time, which is the one DNS rebinding
// cannot race.
type GuardedDialer struct {
	guard   *Guard
	timeout time.Duration
}

// NewGuardedDialer builds the dialler. A zero timeout falls back to ten seconds (rule 7).
func NewGuardedDialer(guard *Guard, timeout time.Duration) GuardedDialer {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return GuardedDialer{guard: guard, timeout: timeout}
}

// DialContext opens a guarded TCP connection to host:port. A refusal by the guard is a coded
// *shared.Error; a failed connection is the network's error, for the adapter to translate.
func (d GuardedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d.guard == nil {
		return nil, shared.ErrInternal.WithDetail(codeDialerUnwired)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, blockedError(codeTargetMalformed, address)
	}
	if err := d.guard.CheckHost(host); err != nil {
		return nil, err
	}
	if _, err := d.guard.Resolve(ctx, host); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{
		Timeout: d.timeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			return d.guard.CheckControl(network, address)
		},
	}
	return dialer.DialContext(ctx, network, address)
}

// DialTLSContext is DialContext followed by a TLS handshake bounded by the same timeout.
func (d GuardedDialer) DialTLSContext(
	ctx context.Context, network, address string, config *tls.Config,
) (net.Conn, error) {
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	client := tls.Client(conn, config)
	if err := client.HandshakeContext(handshakeCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}
