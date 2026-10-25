// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox_test

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/mailbox"
)

type reader struct{}
type session struct{}

func (reader) Open(context.Context, mailbox.Connection) (mailbox.Session, error) {
	return session{}, nil
}
func (session) Fetch(context.Context, int) ([]mailbox.Message, error) { return nil, nil }
func (session) Done(context.Context, string) error                    { return nil }
func (session) Close() error                                          { return nil }

// The port is two small interfaces, which a test double can be in four lines.
func TestTheReaderIsSmall(t *testing.T) {
	var r mailbox.Reader = reader{}
	s, err := r.Open(context.Background(), mailbox.Connection{})
	if err != nil || s.Close() != nil {
		t.Fatal(err)
	}
}
