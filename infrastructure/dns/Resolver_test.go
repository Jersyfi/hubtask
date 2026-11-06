// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package dns

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A name that does not exist is an answer; a timeout or a failing server is not, and the name
// asked never reaches the error.
func TestALookupTellsAnAnswerFromNone(t *testing.T) {
	if err := classify(&net.DNSError{Err: "no such host", Name: "tasks.acme.com", IsNotFound: true}); err != nil {
		t.Errorf("not found: %v", err)
	}
	for _, failed := range []error{
		&net.DNSError{Err: "i/o timeout", Name: "tasks.acme.com", IsTimeout: true},
		&net.DNSError{Err: "server misbehaving", Name: "tasks.acme.com", IsTemporary: true},
		context.DeadlineExceeded,
	} {
		err := classify(failed)
		if !errors.Is(err, shared.ErrUnavailable) || strings.Contains(err.Error(), "acme") {
			t.Errorf("classify(%v) = %v", failed, err)
		}
	}
}

// Rule 7: every lookup carries a deadline, whatever the caller's context says. A resolver whose
// server never answers is cut off at the configured timeout, without the real network.
func TestEveryLookupHasADeadline(t *testing.T) {
	never := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	started := time.Now()
	_, err := Resolver{Timeout: 50 * time.Millisecond, Net: never}.TXT(context.Background(), "_hubtask.tasks.acme.com")
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("the lookup ran %v", elapsed)
	}
}
