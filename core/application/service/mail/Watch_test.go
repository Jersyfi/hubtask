// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type idleServer struct {
	arrived bool
	err     error
	held    []mailboxport.Connection
}

func (s *idleServer) Watch(_ context.Context, connection mailboxport.Connection, _ time.Duration) (bool, error) {
	s.held = append(s.held, connection)
	return s.arrived, s.err
}

type openKeys []queue.OpenKey

func (k openKeys) OpenKeyCount(context.Context, queue.Kind) (int64, error) { return int64(len(k)), nil }

func (k openKeys) OpenKeys(_ context.Context, kind queue.Kind, _ int) ([]queue.OpenKey, error) {
	if kind != queue.KindMailboxPoll {
		return nil, nil
	}
	return k, nil
}

func TestAnArrivalPullsTheMailboxsReadForward(t *testing.T) {
	b, store, _, jobs := boxes(&authorizer{})
	if _, err := (ConnectMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"preset": "gmail", "address": "familie@gmail.com", "secret": "app-pw"}); err != nil {
		t.Fatal(err)
	}
	seeded := len(jobs.requests)
	server := &idleServer{arrived: true}
	watch := WatchMailbox{Boxes: b, Watcher: server}
	target := WatchTarget{TenantID: tenant, MailboxID: boxID}

	outcome, err := watch.Hold(context.Background(), target, time.Minute)
	if err != nil || outcome != WatchArrived {
		t.Fatalf("an arrival answered %s, %v", outcome, err)
	}
	if len(jobs.requests) != seeded+1 {
		t.Fatalf("an arrival queued %d reads", len(jobs.requests)-seeded)
	}
	read := jobs.requests[len(jobs.requests)-1]
	if read.Kind != queue.KindMailboxPoll || read.DedupeKey != boxID.String() || !read.RunAt.Equal(fixed) {
		t.Errorf("the read queued %+v, want the mailbox's own read now", read)
	}
	if server.held[0].Secret.Reveal() != "app-pw" {
		t.Error("the hold signed in without the mailbox's secret")
	}

	server.arrived = false
	if outcome, _ := watch.Hold(context.Background(), target, time.Minute); outcome != WatchQuiet || len(jobs.requests) != seeded+1 {
		t.Errorf("a quiet hold answered %s and queued %d", outcome, len(jobs.requests)-seeded-1)
	}
	server.err = shared.ErrUnavailable.WithDetail("mail.mailbox_idle_unsupported")
	if outcome, err := watch.Hold(context.Background(), target, time.Minute); outcome != WatchUnsupported || err != nil {
		t.Errorf("a server without IDLE answered %s, %v", outcome, err)
	}
	server.err = shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable")
	if outcome, err := watch.Hold(context.Background(), target, time.Minute); outcome != WatchFailed || shared.AsError(err).DetailCode != "mail.mailbox_unreachable" {
		t.Errorf("a broken hold answered %s, %v", outcome, err)
	}

	server.err = nil
	box := store.boxes[boxID]
	box.Enabled = false
	store.boxes[boxID] = box
	if outcome, _ := watch.Hold(context.Background(), target, time.Minute); outcome != WatchGone {
		t.Errorf("a switched-off mailbox answered %s", outcome)
	}
	box.Enabled, box.Protocol = true, domain.ProtocolJMAP
	store.boxes[boxID] = box
	if outcome, _ := watch.Hold(context.Background(), target, time.Minute); outcome != WatchGone {
		t.Errorf("a JMAP mailbox answered %s", outcome)
	}
	delete(store.boxes, boxID)
	if outcome, _ := watch.Hold(context.Background(), target, time.Minute); outcome != WatchGone {
		t.Errorf("a removed mailbox answered %s", outcome)
	}
}

func TestTheTargetsAreTheMailboxesWhoseReadLives(t *testing.T) {
	b, _, _, _ := boxes(&authorizer{})
	watch := WatchMailbox{Boxes: b, Keys: openKeys{
		{TenantID: tenant, DedupeKey: boxID.String()},
		{TenantID: tenant, DedupeKey: "not an id"},
		{DedupeKey: boxID.String()},
	}}
	targets, err := watch.Targets(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != (WatchTarget{TenantID: tenant, MailboxID: boxID}) {
		t.Errorf("targets %+v", targets)
	}
}
