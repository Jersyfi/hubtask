// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	mailadapter "github.com/Jersyfi/hubtask/infrastructure/mail"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// The acceptance the task names, end to end and against the real pieces: a real PostgreSQL, the
// real queue and its runner, the real SMTP adapter behind the real breaker, the real health
// registry, and the real message catalogue.
//
// What it proves is the sentence observability-reliability.md §7 writes for SMTP: "notifications
// stay in the queue and are caught up; no loss". A mail server that is down must not lose a
// message, must be visible in /meta/health, and must not need a restart to recover from.

// mailServer is a mail server that can be stopped and started again on the same address, which is
// the whole point: a restart of the *dependency* must not need a restart of this process.
type mailServer struct {
	listener net.Listener
	mu       sync.Mutex
	received []string
	refusing bool
	done     chan struct{}
}

func newMailServer(ctx context.Context, t *testing.T) *mailServer {
	t.Helper()

	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	server := &mailServer{listener: listener, done: make(chan struct{})}
	concurrency.Go(ctx, "test.mail_server", func(context.Context) { server.accept() })
	t.Cleanup(func() {
		_ = listener.Close()
		<-server.done
	})
	return server
}

func (s *mailServer) accept() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.converse(conn)
	}
}

// stop and start switch the server between refusing every conversation and holding one. A refusal
// at the greeting rather than a closed listener, so that the address stays the same across the
// outage - which is what a mail server being restarted actually looks like.
func (s *mailServer) stop()  { s.set(true) }
func (s *mailServer) start() { s.set(false) }

func (s *mailServer) set(refusing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusing = refusing
}

func (s *mailServer) down() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refusing
}

func (s *mailServer) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.received...)
}

func (s *mailServer) converse(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	if s.down() {
		write("421 4.3.2 Service not available, closing transmission channel")
		return
	}
	write("220 test.invalid ESMTP")

	reader := bufio.NewReader(conn)
	var body strings.Builder
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.received = append(s.received, body.String())
				s.mu.Unlock()
				body.Reset()
				write("250 2.0.0 Ok")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}

		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			write("250-test.invalid")
			write("250 SIZE 10240000")
		case line == "DATA":
			inData = true
			write("354 End data with <CR><LF>.<CR><LF>")
		case line == "QUIT":
			write("221 2.0.0 Bye")
			return
		default:
			write("250 2.0.0 Ok")
		}
	}
}

// notificationStack is everything the delivery needs, wired the way the composition root wires it.
type notificationStack struct {
	runner   worker.Runner
	registry *healthadapter.Registry
	mail     *mailServer
}

func newNotificationStack(ctx context.Context, t *testing.T) notificationStack {
	t.Helper()

	server := newMailServer(ctx, t)
	host, port := server.listener.Addr().(*net.TCPAddr).IP.String(),
		server.listener.Addr().(*net.TCPAddr).Port

	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: mailadapter.Dependency,
		// Tripped quickly and re-opened quickly. The breaker is not incidental here: the health
		// probe reads it, so "the feature is degraded" and "the breaker is open" are the same
		// fact, and a short cool-down is what lets the recovery happen inside a test rather than
		// inside a coffee break.
		FailureThreshold: 2, SuccessThreshold: 1, OpenFor: 200 * time.Millisecond,
	})
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{
		Name: mailadapter.Dependency, Capacity: 4,
	})
	sender := mailadapter.NewResilientSender(
		mailadapter.NewSMTP(envport.MailConfig{
			Host: host, Port: port, From: "hubtask@test.invalid",
			Security: envport.MailSecurityNone, Timeout: 2 * time.Second,
		}), breaker, bulkhead)

	registry := healthadapter.NewRegistry("test", []string{"worker"})
	registry.Register(mailadapter.NewProbe(breaker, true))
	registry.MarkStarted()

	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	pool := appPool(ctx, t)
	work := postgres.NewUnitOfWork(pool)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	notifications := postgres.NewNotificationRepository()
	preferences := postgres.NewNotificationPreferenceRepository()
	accounts := postgres.NewAccountRepository()
	jobs := postgres.NewQueue(ids, clockadapter.System{})

	handlers := map[queue.Kind]queue.Handler{
		queue.KindInvitationEmail: worker.InvitationMessage{
			Invitation: notification.RecordInvitation{
				Notifications: notifications, Accounts: accounts, Jobs: jobs,
				Clock: clockadapter.System{}, IDs: ids,
			},
		},
		queue.KindNotificationDeliver: worker.NotificationDelivery{
			Delivery: notification.DeliverNotification{
				Notifications: notifications, Preferences: preferences, Accounts: accounts,
				Items: postgres.NewItemRepository(pageCursors()), Mail: sender, Renderer: renderer,
				UnitOfWork: work, Clock: clockadapter.System{},
				BaseURL: "https://hub.test.invalid",
			},
		},
	}

	return notificationStack{
		runner: worker.Runner{
			Queue: jobs, UnitOfWork: work, Handlers: handlers, Clock: clockadapter.System{},
			Batch: 10, PollInterval: 50 * time.Millisecond, JobTimeout: 5 * time.Second,
			Lease: 30 * time.Second,
			// The production backoff in miniature. Not zero: a job's attempt budget is eight, and
			// retrying with no wait at all would spend all eight inside a second - which would be
			// the test proving that an instant retry loop gives up, rather than that a message
			// survives an outage.
			NextAttempt: func(attempt int) time.Duration {
				return time.Duration(attempt) * 150 * time.Millisecond
			},
		},
		registry: registry,
		mail:     server,
	}
}

// seedPerson writes one account with an address and a language.
func seedPerson(ctx context.Context, t *testing.T, tenant shared.ID, name, email, locale string) shared.ID {
	t.Helper()

	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name, email, locale)
		 VALUES ($1, $2, $3, $4, $5)`,
		id.String(), tenant.String(), name, email, locale); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return id
}

// theOnlyNotification returns the tenant's single record, waiting for it to appear.
func theOnlyNotification(
	ctx context.Context, t *testing.T, tenant shared.ID,
) domain.Notification {
	t.Helper()

	var id shared.ID
	waitFor(t, 5*time.Second, "a notification record", func() bool {
		var found string
		err := adminPool(ctx, t).QueryRow(ctx,
			"SELECT id::text FROM notification WHERE tenant_id = $1", tenant.String()).Scan(&found)
		if err != nil {
			return false
		}
		id = shared.MustParseID(found)
		return true
	})

	record, err := findNotification(ctx, t, tenant, id)
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	return record
}

// waitFor polls until the condition holds, which is how a test watches a loop it does not drive.
func waitFor(t *testing.T, within time.Duration, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("gave up waiting for %s after %v", what, within)
}

// The acceptance criterion in one test: with SMTP down the notification stays queued, the health
// report says the feature is degraded, and the queue catches up on its own when the server
// returns - without anything being restarted.
func TestANotificationSurvivesAMailServerOutage(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	stack := newNotificationStack(ctx, t)
	stack.mail.stop()

	inviter := seedPerson(ctx, t, tenantA, "Anna", freshName(t)+"@test.invalid", "de")
	invited := seedPerson(ctx, t, tenantA, "Bert", freshName(t)+"@test.invalid", "en")

	// The worker runs for the whole test. Nothing here claims or completes a job by hand: what is
	// under test is the queue catching up on its own.
	running, stop := context.WithCancel(ctx)
	defer stop()
	concurrency.Go(running, "test.worker", stack.runner.Run)

	enqueue(ctx, t, queue.Request{
		Kind: queue.KindInvitationEmail, TenantID: tenantA,
		DedupeKey: invited.String(),
		Payload: map[string]any{
			"account_id": invited.String(), "invited_by": inviter.String(),
		},
	})

	// --- The mail server is down -----------------------------------------------------------
	record := theOnlyNotification(ctx, t, tenantA)
	if record.Category != domain.CategoryInvitation {
		t.Errorf("category %q, want the invitation", record.Category)
	}

	// It is tried, and it stays pending: the message is not lost and it is not given up on. The
	// outage is reported rather than hidden at the same moment, because the health probe reads
	// the breaker the failing attempts trip.
	var report healthport.Report
	waitFor(t, 5*time.Second, "a delivery attempt and a degraded report", func() bool {
		current, err := findNotification(ctx, t, tenantA, record.ID)
		if err != nil || current.Attempts == 0 {
			return false
		}
		report = stack.registry.Report(ctx)
		return report.Status == healthport.StatusDegraded
	})

	current, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if current.State != domain.StatePending {
		t.Errorf("state %q during the outage, want it still pending - the message must not be lost",
			current.State)
	}
	if len(stack.mail.messages()) != 0 {
		t.Fatal("a message got through a mail server that is refusing every connection")
	}

	var degraded bool
	for _, feature := range report.DegradedFeatures {
		if feature.Feature == mailadapter.NotificationsFeature {
			degraded = true
		}
	}
	if !degraded {
		t.Errorf("degraded features %v, want %s", report.DegradedFeatures,
			mailadapter.NotificationsFeature)
	}
	if ready, reason := stack.registry.Ready(ctx); !ready {
		t.Errorf("the process reported itself unready over the mail server: %s", reason)
	}

	// --- The mail server comes back --------------------------------------------------------
	// Nothing is restarted and nothing is reconfigured. The dependency returns on the same
	// address, which is what a mail server being restarted actually looks like.
	stack.mail.start()

	waitFor(t, 15*time.Second, "the queue to catch up", func() bool {
		sent, err := findNotification(ctx, t, tenantA, record.ID)
		return err == nil && sent.State == domain.StateSent
	})

	messages := stack.mail.messages()
	if len(messages) != 1 {
		t.Fatalf("%d messages arrived, want the one that was waiting", len(messages))
	}
	if !strings.Contains(messages[0], "https://hub.test.invalid") {
		t.Errorf("the message carries no link:\n%s", messages[0])
	}
	// Bert's language, not Anna's: the recipient's locale decides (i18n-l10n.md §1).
	if strings.Contains(messages[0], "email.invitation") {
		t.Errorf("the message was not rendered at all:\n%s", messages[0])
	}

	sent, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if sent.SentAt == nil {
		t.Error("the record says sent and does not say when")
	}
	if sent.Attempts < 1 {
		t.Errorf("attempts %d - the failed attempt was not counted", sent.Attempts)
	}
}
